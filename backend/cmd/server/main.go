package main

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/bharatchat/backend/internal/config"
	authrepo "github.com/bharatchat/backend/internal/features/auth/repository"
	authservice "github.com/bharatchat/backend/internal/features/auth/service"
	authtransport "github.com/bharatchat/backend/internal/features/auth/transport"

	chatrepo "github.com/bharatchat/backend/internal/features/chat/repository"
	chatservice "github.com/bharatchat/backend/internal/features/chat/service"
	chattransport "github.com/bharatchat/backend/internal/features/chat/transport"
	grouprepo "github.com/bharatchat/backend/internal/features/group/repository"
	groupservice "github.com/bharatchat/backend/internal/features/group/service"
	grouptransport "github.com/bharatchat/backend/internal/features/group/transport"

	messagerepo "github.com/bharatchat/backend/internal/features/message/repository"
	messageservice "github.com/bharatchat/backend/internal/features/message/service"
	messagetransport "github.com/bharatchat/backend/internal/features/message/transport"

	presenceservice "github.com/bharatchat/backend/internal/features/presence/service"
	privacyrepo "github.com/bharatchat/backend/internal/features/privacy/repository"
	privacyservice "github.com/bharatchat/backend/internal/features/privacy/service"
	privacytransport "github.com/bharatchat/backend/internal/features/privacy/transport"

	userrepo "github.com/bharatchat/backend/internal/features/user/repository"
	userservice "github.com/bharatchat/backend/internal/features/user/service"
	usertransport "github.com/bharatchat/backend/internal/features/user/transport"

	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/internal/platform/cache"
	"github.com/bharatchat/backend/internal/platform/database"
	"github.com/bharatchat/backend/internal/platform/health"
	"github.com/bharatchat/backend/internal/platform/logger"
	"github.com/bharatchat/backend/internal/platform/otpdelivery"
	"github.com/bharatchat/backend/internal/platform/validator"
	wsplatform "github.com/bharatchat/backend/internal/platform/websocket"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	log := logger.New(cfg.LogLevel, cfg.AppEnv)
	log.Info().Str("env", cfg.AppEnv).Msg("starting bharatchat backend")

	if err := database.RunMigrations(cfg.Postgres.DSN(), "./migrations"); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	pool, err := database.NewPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pool.Close()
	database.StartPrivacyCleanup(ctx, pool, log)

	rdb, err := cache.NewClient(ctx, cfg.Redis.Addr(), cfg.Redis.Password, cfg.Redis.UseTLS, cfg.Redis.TLSServerName)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer rdb.Close()

	v := validator.New()

	// --- WebSocket hub (must exist before any feature that publishes through it) ---
	hub := wsplatform.NewHub(rdb, log)
	if err := hub.StartSubscriber(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start websocket subscriber")
	}
	hubPublisher := &wsplatform.HubEventPublisherAdapter{Hub: hub}

	// --- user feature wiring (Module 2) ---
	userRepository := userrepo.NewUserPostgresRepository(pool)
	userSvc := userservice.NewUserService(userRepository)
	userHandler := usertransport.NewUserHandler(userSvc, v)
	userHandler.WithSafetyLimits(
		middleware.RateLimit(rdb, "username-lookup", 20, time.Minute, middleware.ByAuthenticatedUser),
		middleware.RateLimit(rdb, "user-report", 10, time.Hour, middleware.ByAuthenticatedUser),
	)
	userHandler.WithAccountExportLimit(middleware.RateLimit(rdb, "account-export", 2, time.Hour, middleware.ByAuthenticatedUser))

	// --- auth feature wiring (Module 2) ---
	otpRepository := authrepo.NewOTPPostgresRepository(pool)
	sessionRepository := authrepo.NewSessionPostgresRepository(pool)
	deviceRepository := authrepo.NewDevicePostgresRepository(pool)
	otpRateLimiter := authrepo.NewOTPRateLimiterRedis(rdb, cfg.OTP.RequestRateLimit, cfg.OTP.RequestRateWindow)
	tokenBlacklist := authrepo.NewTokenBlacklistRedis(rdb)
	wsTicketStore := authrepo.NewWebSocketTicketRedis(rdb)
	tokenSvc := authservice.NewTokenService(
		cfg.JWT.AccessSecret, cfg.JWT.AccessTTL, cfg.JWT.Issuer, cfg.JWT.Audience,
	)
	var otpDelivery authservice.OTPDelivery
	switch cfg.OTP.DeliveryMode {
	case "msg91":
		otpDelivery = otpdelivery.NewMSG91(cfg.OTP.MSG91AuthKey, cfg.OTP.MSG91TemplateID, cfg.OTP.TTL, nil)
	case "webhook":
		otpDelivery = otpdelivery.NewWebhook(cfg.OTP.WebhookURL, cfg.OTP.WebhookBearerToken, nil)
	case "twilio":
		otpDelivery = otpdelivery.NewTwilioMessaging(
			cfg.OTP.TwilioAccountSID,
			cfg.OTP.TwilioAPIKey,
			cfg.OTP.TwilioAPISecret,
			cfg.OTP.TwilioMessagingServiceSID,
			cfg.OTP.SMSMessageTemplate,
			cfg.OTP.TTL,
			nil,
		)
	default:
		otpDelivery = authservice.OTPDeliveryFunc(func(_ context.Context, phoneNumber, code string) error {
			fmt.Printf("[DEV-ONLY OTP DELIVERY] phone=%s code=%s\n", phoneNumber, code)
			return nil
		})
	}

	authOptions := []authservice.AuthServiceOption{authservice.WithOTPDelivery(otpDelivery)}
	if cfg.AppEnv != "development" || len(cfg.OTP.AllowedPhones) > 0 {
		authOptions = append(authOptions, authservice.WithAllowedPhones(cfg.OTP.AllowedPhones))
	}
	authSvc := authservice.NewAuthService(
		otpRepository, sessionRepository, deviceRepository, tokenBlacklist, otpRateLimiter, userSvc,
		tokenSvc, tokenSvc,
		authservice.AuthServiceConfig{
			OTPTTL:         cfg.OTP.TTL,
			OTPMaxAttempts: cfg.OTP.MaxAttempts,
			RefreshTTLDays: cfg.JWT.RefreshTTLDays,
		},
		authOptions...,
	)
	userSvc.UseAccountDeletionAuthorizer(authSvc)
	authHandler := authtransport.NewAuthHandler(authSvc, tokenSvc, v, cfg.JWT.AccessTTL, wsTicketStore).
		StoreNetworkMetadata(cfg.Privacy.StoreSessionNetworkMetadata)

	privacyRepository := privacyrepo.NewKeyPostgresRepository(pool)
	privacySvc := privacyservice.NewPrivacyService(privacyRepository)
	privacyHandler := privacytransport.NewPrivacyHandler(privacySvc, v)

	// --- chat feature wiring (Module 3, new) ---
	chatRepository := chatrepo.NewChatPostgresRepository(pool)
	chatSvc := chatservice.NewChatService(chatRepository, userSvc)
	chatHandler := chattransport.NewChatHandler(chatSvc, v)
	groupRepository := grouprepo.NewGroupPostgresRepository(pool)
	groupSvc := groupservice.NewGroupService(groupRepository, chatSvc, userSvc)
	groupHandler := grouptransport.NewGroupHandler(groupSvc, v)

	// Adapters bridging chat -> message without a package cycle (see §2.9 adapters).
	chatAuthAdapter := &messageservice.ChatServiceAuthorizerAdapter{
		AuthorizeFn:   chatSvc.AuthorizeParticipant,
		InteractionFn: chatSvc.AuthorizeInteraction,
	}
	chatParticipantAdapter := &messageservice.ChatParticipantListerAdapter{
		ListParticipantsFn: chatRepository.ListParticipants,
	}

	// --- message feature wiring (Module 3, new) ---
	messageRepository := messagerepo.NewMessagePostgresRepository(pool)
	messageSvc := messageservice.NewMessageService(messageRepository, chatAuthAdapter, chatParticipantAdapter, hubPublisher, groupSvc)
	messageSvc.RequireEncryption(cfg.Privacy.RequireE2EE)
	messageSvc.SetMessagingEnabled(cfg.MessagingEnabled)
	messageSvc.UseReadReceiptPolicy(userSvc)
	messageHandler := messagetransport.NewMessageHandler(messageSvc)

	// --- presence feature wiring (Module 3, new) ---
	presenceSvc := presenceservice.NewPresenceService(rdb, hubPublisher, chatParticipantAdapter)
	presenceSvc.UseTypingPrivacyPolicy(userSvc)

	messageWSHandler := messagetransport.NewMessageWSHandler(hub, messageSvc, presenceSvc, log, cfg.CORSAllowedOrigins...).
		WithSessionDeviceResolver(privacySvc)

	// --- HTTP server wiring ---
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatal().Err(err).Msg("invalid trusted proxy configuration")
	}
	router.Use(middleware.Recovery(log))
	router.Use(middleware.RequestID())
	router.Use(middleware.RequestLogger(log))
	router.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	router.Use(middleware.SecurityHeaders(cfg.AppEnv == "production"))
	router.Use(middleware.LimitRequestBody())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/readyz", health.Readiness(log, 2*time.Second, map[string]health.Check{
		"postgres": pool.Ping,
		"redis": func(ctx context.Context) error {
			return rdb.Ping(ctx).Err()
		},
	}))

	api := router.Group("/api/v1")

	otpLimiter := middleware.RateLimit(rdb, "otp", 10, 10*time.Minute, middleware.ByIP)
	api.POST("/auth/otp/request", otpLimiter, authHandler.RequestOTP)
	api.POST("/auth/otp/verify", otpLimiter, authHandler.VerifyOTP)
	refreshLimiter := middleware.RateLimit(rdb, "refresh", 30, 10*time.Minute, middleware.ByIP)
	api.POST("/auth/refresh", refreshLimiter, authHandler.RefreshToken)

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(tokenSvc, tokenBlacklist, sessionRepository))
	protected.Use(middleware.RateLimit(rdb, "authenticated", 600, time.Minute, middleware.ByAuthenticatedUser))

	authHandler.RegisterProtectedRoutes(protected)
	userHandler.RegisterRoutes(protected)
	chatHandler.RegisterRoutes(protected)
	groupHandler.RegisterRoutes(protected)
	messageHandler.RegisterRoutes(protected)
	privacyHandler.RegisterRoutes(protected)

	webSocketRoutes := api.Group("")
	webSocketRoutes.Use(middleware.RequireWebSocketTicket(wsTicketStore))
	messageWSHandler.RegisterRoutes(webSocketRoutes)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	go func() {
		log.Info().Str("port", cfg.HTTPPort).Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received, draining connections")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
}
