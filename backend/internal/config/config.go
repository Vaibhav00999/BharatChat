package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type PostgresConfig struct{ Host, Port, User, Password, DBName, SSLMode, SSLRootCert string }

func (p PostgresConfig) DSN() string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   net.JoinHostPort(p.Host, p.Port),
		Path:   "/" + p.DBName,
	}
	query := dsn.Query()
	query.Set("sslmode", p.SSLMode)
	if p.SSLRootCert != "" {
		query.Set("sslrootcert", p.SSLRootCert)
	}
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

type RedisConfig struct {
	Host, Port, Password string
	UseTLS               bool
	TLSServerName        string
}

func (r RedisConfig) Addr() string { return fmt.Sprintf("%s:%s", r.Host, r.Port) }

type JWTConfig struct {
	AccessSecret   string
	AccessTTL      time.Duration
	RefreshTTLDays int
	Issuer         string
	Audience       string
}
type OTPConfig struct {
	TTL                           time.Duration
	MaxAttempts, RequestRateLimit int
	RequestRateWindow             time.Duration
	DeliveryMode                  string
	WebhookURL                    string
	WebhookBearerToken            string
	TwilioAccountSID              string
	TwilioAPIKey                  string
	TwilioAPISecret               string
	TwilioMessagingServiceSID     string
	SMSMessageTemplate            string
	MSG91AuthKey                  string
	MSG91TemplateID               string
	AllowedPhones                 []string
}
type PrivacyConfig struct {
	RequireE2EE                 bool
	StoreSessionNetworkMetadata bool
}
type Config struct {
	AppEnv, HTTPPort, LogLevel string
	Postgres                   PostgresConfig
	Redis                      RedisConfig
	JWT                        JWTConfig
	OTP                        OTPConfig
	Privacy                    PrivacyConfig
	CORSAllowedOrigins         []string
	TrustedProxies             []string
	MessagingEnabled           bool
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func Load() (*Config, error) {
	appEnv := strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", "development")))
	accessSecret := os.Getenv("JWT_ACCESS_SECRET")
	postgresPassword := getEnv("POSTGRES_PASSWORD", "")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	if accessSecret == "" {
		return nil, fmt.Errorf("config: JWT_ACCESS_SECRET is required but not set")
	}
	if appEnv == "production" && (len(accessSecret) < 32 || accessSecret == "change_this_access_secret_in_prod") {
		return nil, fmt.Errorf("config: production JWT_ACCESS_SECRET must be a non-default secret of at least 32 characters")
	}
	postgresSSLMode := strings.ToLower(strings.TrimSpace(getEnv("POSTGRES_SSLMODE", "disable")))
	if appEnv == "production" && postgresSSLMode != "verify-full" {
		return nil, fmt.Errorf("config: production POSTGRES_SSLMODE must be verify-full")
	}
	if appEnv == "production" && postgresPassword == "" {
		return nil, fmt.Errorf("config: POSTGRES_PASSWORD is required in production")
	}
	redisTLS := getEnvBool("REDIS_TLS", false)
	if appEnv == "production" && !redisTLS {
		return nil, fmt.Errorf("config: REDIS_TLS must be enabled in production")
	}
	if appEnv == "production" && redisPassword == "" {
		return nil, fmt.Errorf("config: REDIS_PASSWORD is required in production")
	}
	origins := strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "*"), ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	if appEnv == "production" && len(origins) == 1 && origins[0] == "*" {
		return nil, fmt.Errorf("config: production CORS_ALLOWED_ORIGINS must list explicit origins")
	}
	trustedProxies := make([]string, 0)
	if raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); raw != "" {
		for _, proxy := range strings.Split(raw, ",") {
			if value := strings.TrimSpace(proxy); value != "" {
				trustedProxies = append(trustedProxies, value)
			}
		}
	}
	requireE2EE := getEnvBool("REQUIRE_E2EE", appEnv == "production")
	if appEnv == "production" && !requireE2EE {
		return nil, fmt.Errorf("config: REQUIRE_E2EE cannot be disabled in production")
	}
	otpDeliveryMode := strings.ToLower(strings.TrimSpace(getEnv("OTP_DELIVERY_MODE", "console")))
	if otpDeliveryMode != "console" && otpDeliveryMode != "webhook" && otpDeliveryMode != "twilio" && otpDeliveryMode != "msg91" {
		return nil, fmt.Errorf("config: OTP_DELIVERY_MODE must be console, webhook, twilio, or msg91")
	}
	otpWebhookURL := strings.TrimSpace(os.Getenv("OTP_WEBHOOK_URL"))
	otpWebhookBearerToken := strings.TrimSpace(os.Getenv("OTP_WEBHOOK_BEARER_TOKEN"))
	if appEnv != "development" && otpDeliveryMode == "console" {
		return nil, fmt.Errorf("config: non-development OTP_DELIVERY_MODE must be webhook, twilio, or msg91")
	}
	if otpDeliveryMode == "webhook" {
		parsedURL, err := url.Parse(otpWebhookURL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
			return nil, fmt.Errorf("config: OTP_WEBHOOK_URL must be an absolute HTTPS URL")
		}
		if len(otpWebhookBearerToken) < 32 {
			return nil, fmt.Errorf("config: OTP_WEBHOOK_BEARER_TOKEN must be at least 32 characters")
		}
	}
	otpTwilioAccountSID := strings.TrimSpace(os.Getenv("OTP_TWILIO_ACCOUNT_SID"))
	msg91AuthKey := strings.TrimSpace(os.Getenv("OTP_MSG91_AUTH_KEY"))
	msg91TemplateID := strings.TrimSpace(os.Getenv("OTP_MSG91_TEMPLATE_ID"))
	if otpDeliveryMode == "msg91" {
		if len(msg91AuthKey) < 16 || strings.ContainsAny(msg91AuthKey, "\r\n\t ") {
			return nil, fmt.Errorf("config: OTP_MSG91_AUTH_KEY must be configured")
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9]{10,64}$`).MatchString(msg91TemplateID) {
			return nil, fmt.Errorf("config: OTP_MSG91_TEMPLATE_ID must be a valid MSG91 OTP template ID")
		}
	}
	otpTwilioAPIKey := strings.TrimSpace(os.Getenv("OTP_TWILIO_API_KEY"))
	otpTwilioAPISecret := strings.TrimSpace(os.Getenv("OTP_TWILIO_API_SECRET"))
	otpTwilioMessagingServiceSID := strings.TrimSpace(os.Getenv("OTP_TWILIO_MESSAGING_SERVICE_SID"))
	otpSMSMessageTemplate := strings.TrimSpace(os.Getenv("OTP_SMS_MESSAGE_TEMPLATE"))
	if otpDeliveryMode == "twilio" {
		if !regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`).MatchString(otpTwilioAccountSID) {
			return nil, fmt.Errorf("config: OTP_TWILIO_ACCOUNT_SID must be a valid Account SID")
		}
		if !regexp.MustCompile(`^SK[0-9a-fA-F]{32}$`).MatchString(otpTwilioAPIKey) {
			return nil, fmt.Errorf("config: OTP_TWILIO_API_KEY must be a valid API key SID")
		}
		if len(otpTwilioAPISecret) < 32 {
			return nil, fmt.Errorf("config: OTP_TWILIO_API_SECRET must be at least 32 characters")
		}
		if !regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`).MatchString(otpTwilioMessagingServiceSID) {
			return nil, fmt.Errorf("config: OTP_TWILIO_MESSAGING_SERVICE_SID must be a valid Messaging Service SID")
		}
		if strings.Count(otpSMSMessageTemplate, "{{CODE}}") != 1 {
			return nil, fmt.Errorf("config: OTP_SMS_MESSAGE_TEMPLATE must contain {{CODE}} exactly once")
		}
		if len(otpSMSMessageTemplate) > 1600 {
			return nil, fmt.Errorf("config: OTP_SMS_MESSAGE_TEMPLATE must not exceed 1600 bytes")
		}
	}
	accessTTLMinutes := getEnvInt("JWT_ACCESS_TTL_MINUTES", 15)
	refreshTTLDays := getEnvInt("JWT_REFRESH_TTL_DAYS", 30)
	otpTTLMinutes := getEnvInt("OTP_TTL_MINUTES", 5)
	otpMaxAttempts := getEnvInt("OTP_MAX_ATTEMPTS", 5)
	otpRequestLimit := getEnvInt("OTP_REQUEST_RATE_LIMIT", 5)
	otpRequestWindowMinutes := getEnvInt("OTP_REQUEST_RATE_WINDOW_MINUTES", 10)
	if accessTTLMinutes < 1 || accessTTLMinutes > 60 {
		return nil, fmt.Errorf("config: JWT_ACCESS_TTL_MINUTES must be between 1 and 60")
	}
	if refreshTTLDays < 1 || refreshTTLDays > 90 {
		return nil, fmt.Errorf("config: JWT_REFRESH_TTL_DAYS must be between 1 and 90")
	}
	if otpTTLMinutes < 1 || otpTTLMinutes > 15 || otpMaxAttempts < 1 || otpMaxAttempts > 10 {
		return nil, fmt.Errorf("config: OTP TTL or attempt limit is outside the permitted range")
	}
	if otpRequestLimit < 1 || otpRequestWindowMinutes < 1 {
		return nil, fmt.Errorf("config: OTP request rate-limit values must be positive")
	}
	allowedPhones, err := parseAllowedPhones(os.Getenv("OTP_ALLOWED_PHONES"))
	if err != nil {
		return nil, err
	}
	if appEnv != "development" && len(allowedPhones) == 0 {
		return nil, fmt.Errorf("config: OTP_ALLOWED_PHONES is required for the controlled pilot")
	}
	messagingEnabled := appEnv == "development"
	if raw, supplied := os.LookupEnv("MESSAGING_ENABLED"); supplied {
		if raw != "true" && raw != "false" {
			return nil, fmt.Errorf("config: MESSAGING_ENABLED must be true or false")
		}
		messagingEnabled = raw == "true"
	}
	if appEnv != "development" && messagingEnabled {
		return nil, fmt.Errorf("config: messaging cannot be enabled outside development until the E2EE launch gate is reviewed")
	}
	return &Config{
		AppEnv: appEnv, HTTPPort: getEnv("HTTP_PORT", "8080"), LogLevel: getEnv("LOG_LEVEL", "info"),
		Postgres: PostgresConfig{
			Host: getEnv("POSTGRES_HOST", "localhost"), Port: getEnv("POSTGRES_PORT", "5432"),
			User: getEnv("POSTGRES_USER", "bharatchat"), Password: postgresPassword,
			DBName: getEnv("POSTGRES_DB", "bharatchat"), SSLMode: postgresSSLMode,
			SSLRootCert: strings.TrimSpace(os.Getenv("POSTGRES_SSLROOTCERT")),
		},
		Redis: RedisConfig{
			Host: getEnv("REDIS_HOST", "localhost"), Port: getEnv("REDIS_PORT", "6379"),
			Password: redisPassword, UseTLS: redisTLS,
			TLSServerName: strings.TrimSpace(os.Getenv("REDIS_TLS_SERVER_NAME")),
		},
		JWT: JWTConfig{
			AccessSecret: accessSecret, AccessTTL: time.Duration(accessTTLMinutes) * time.Minute,
			RefreshTTLDays: refreshTTLDays, Issuer: getEnv("JWT_ISSUER", "bharatchat-api"),
			Audience: getEnv("JWT_AUDIENCE", "bharatchat-clients"),
		},
		OTP: OTPConfig{
			TTL: time.Duration(otpTTLMinutes) * time.Minute, MaxAttempts: otpMaxAttempts,
			RequestRateLimit: otpRequestLimit, RequestRateWindow: time.Duration(otpRequestWindowMinutes) * time.Minute,
			DeliveryMode: otpDeliveryMode, WebhookURL: otpWebhookURL, WebhookBearerToken: otpWebhookBearerToken,
			TwilioAccountSID: otpTwilioAccountSID, TwilioAPIKey: otpTwilioAPIKey,
			TwilioAPISecret: otpTwilioAPISecret, TwilioMessagingServiceSID: otpTwilioMessagingServiceSID,
			SMSMessageTemplate: otpSMSMessageTemplate,
			MSG91AuthKey:       msg91AuthKey, MSG91TemplateID: msg91TemplateID,
			AllowedPhones: allowedPhones,
		},
		Privacy:            PrivacyConfig{RequireE2EE: requireE2EE, StoreSessionNetworkMetadata: getEnvBool("STORE_SESSION_NETWORK_METADATA", false)},
		CORSAllowedOrigins: origins,
		TrustedProxies:     trustedProxies,
		MessagingEnabled:   messagingEnabled,
	}, nil
}

// Keep the initial SMS test cohort small; never include phone values in errors.
func parseAllowedPhones(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	phones := strings.Split(raw, ",")
	if len(phones) > 20 {
		return nil, fmt.Errorf("config: OTP_ALLOWED_PHONES cannot exceed 20 test numbers")
	}
	e164 := regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	seen := make(map[string]bool, len(phones))
	for _, phone := range phones {
		if !e164.MatchString(phone) || seen[phone] {
			return nil, fmt.Errorf("config: OTP_ALLOWED_PHONES must contain unique E.164 numbers separated by commas without spaces")
		}
		seen[phone] = true
	}
	return phones, nil
}
