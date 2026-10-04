// backend/internal/features/auth/transport/auth_integration_test.go
package transport_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authrepo "github.com/bharatchat/backend/internal/features/auth/repository"
	authservice "github.com/bharatchat/backend/internal/features/auth/service"
	authtransport "github.com/bharatchat/backend/internal/features/auth/transport"
	userrepo "github.com/bharatchat/backend/internal/features/user/repository"
	userservice "github.com/bharatchat/backend/internal/features/user/service"
	usertransport "github.com/bharatchat/backend/internal/features/user/transport"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/internal/platform/database"
	"github.com/bharatchat/backend/internal/platform/validator"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestServer(t *testing.T, delivery ...authservice.OTPDelivery) (*httptest.Server, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()

	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "test",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })

	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(context.Background()) })

	pgHost, err := pgContainer.Host(ctx)
	require.NoError(t, err)
	pgPort, err := pgContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)
	dsn := fmt.Sprintf("postgres://test:test@%s:%s/test?sslmode=disable", pgHost, pgPort.Port())

	require.NoError(t, database.RunMigrations(dsn, "../../../../migrations"))

	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", redisHost, redisPort.Port())})

	router := buildTestRouter(pool, rdb, delivery...)
	server := httptest.NewServer(router)

	cleanup := func() {
		server.Close()
		pool.Close()
		rdb.Close()
	}

	return server, cleanup
}

func buildTestRouter(pool *pgxpool.Pool, rdb *redis.Client, delivery ...authservice.OTPDelivery) *gin.Engine {
	gin.SetMode(gin.TestMode)
	v := validator.New()

	userRepository := userrepo.NewUserPostgresRepository(pool)
	userSvc := userservice.NewUserService(userRepository)
	userHandler := usertransport.NewUserHandler(userSvc, v)

	otpRepository := authrepo.NewOTPPostgresRepository(pool)
	sessionRepository := authrepo.NewSessionPostgresRepository(pool)
	deviceRepository := authrepo.NewDevicePostgresRepository(pool)
	rateLimiter := authrepo.NewOTPRateLimiterRedis(rdb, 100, time.Hour) // generous limit for tests
	blacklist := authrepo.NewTokenBlacklistRedis(rdb)
	tokenSvc := authservice.NewTokenService("integration-test-secret", 15*time.Minute)

	var sender authservice.OTPDelivery = authservice.OTPDeliveryFunc(func(context.Context, string, string) error { return nil })
	if len(delivery) > 0 {
		sender = delivery[0]
	}
	authSvc := authservice.NewAuthService(
		otpRepository, sessionRepository, deviceRepository, blacklist, rateLimiter, userSvc,
		tokenSvc, tokenSvc,
		authservice.AuthServiceConfig{OTPTTL: 5 * time.Minute, OTPMaxAttempts: 5, RefreshTTLDays: 30},
		authservice.WithOTPDelivery(sender),
	)
	userSvc.UseAccountDeletionAuthorizer(authSvc)
	authHandler := authtransport.NewAuthHandler(authSvc, tokenSvc, v, 15*time.Minute)

	router := gin.New()
	api := router.Group("/api/v1")
	authHandler.RegisterPublicRoutes(api)

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(tokenSvc, blacklist, sessionRepository))
	authHandler.RegisterProtectedRoutes(protected)
	userHandler.RegisterRoutes(protected)

	return router
}

func postJSON(t *testing.T, url string, body interface{}, headers map[string]string) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestFullAuthFlow_RequestVerifyRefreshLogout drives the entire OTP -> JWT lifecycle
// through real HTTP calls against real Postgres + Redis, proving every layer wires
// together correctly end-to-end (this is intentionally the single most important
// test in this module).
func TestFullAuthFlow_RequestVerifyRefreshLogout(t *testing.T) {
	codes := make(chan string, 4)
	server, cleanup := setupTestServer(t, authservice.OTPDeliveryFunc(func(_ context.Context, _, code string) error { codes <- code; return nil }))
	defer cleanup()

	phone := "+919876500099"

	// Step 1: request OTP
	resp := postJSON(t, server.URL+"/api/v1/auth/otp/request", map[string]string{
		"phoneNumber": phone, "countryCode": "+91",
	}, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	code := <-codes
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "111111"
	}
	resp = postJSON(t, server.URL+"/api/v1/auth/otp/verify", map[string]interface{}{
		"phoneNumber": phone, "countryCode": "+91", "code": wrongCode,
		"device": map[string]string{"platform": "android", "deviceName": "Test Device", "appVersion": "1.0.0"},
	}, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	verify := map[string]interface{}{
		"phoneNumber": phone, "countryCode": "+91", "code": code,
		"device": map[string]string{"platform": "android", "deviceName": "Test Device", "appVersion": "1.0.0"},
	}
	resp = postJSON(t, server.URL+"/api/v1/auth/otp/verify", verify, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var session authtransport.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&session))
	resp.Body.Close()
	require.NotEmpty(t, session.UserID)
	require.True(t, session.IsNewUser)
	resp = postJSON(t, server.URL+"/api/v1/auth/otp/verify", verify, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	resp = postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{"refreshToken": session.RefreshToken}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var rotated authtransport.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rotated))
	resp.Body.Close()
	require.NotEqual(t, session.RefreshToken, rotated.RefreshToken)
	resp = postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{"refreshToken": session.RefreshToken}, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	resp = requestJSON(t, http.MethodGet, server.URL+"/api/v1/users/me", nil, rotated.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	resp = requestJSON(t, http.MethodPost, server.URL+"/api/v1/auth/logout", nil, rotated.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	for _, token := range []string{session.AccessToken, rotated.AccessToken} {
		resp = requestJSON(t, http.MethodGet, server.URL+"/api/v1/users/me", nil, token)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		resp.Body.Close()
	}
	resp = postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{"refreshToken": rotated.RefreshToken}, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func requestJSON(t *testing.T, method, endpoint string, body interface{}, token string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestRefreshToken_InvalidToken_Returns401 verifies the refresh endpoint rejects
// tokens that don't correspond to any session.
func TestRefreshToken_InvalidToken_Returns401(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	resp := postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{
		"refreshToken": "clearly-not-a-real-token",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

// TestProtectedRoute_WithoutToken_Returns401 verifies RequireAuth middleware is
// actually mounted on protected routes.
func TestProtectedRoute_WithoutToken_Returns401(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/api/v1/users/me")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestDeleteAccount_RequiresCurrentRefreshCredentialAndRevokesSession(t *testing.T) {
	codes := make(chan string, 1)
	server, cleanup := setupTestServer(t, authservice.OTPDeliveryFunc(func(_ context.Context, _, code string) error {
		codes <- code
		return nil
	}))
	defer cleanup()

	phone := "+919876500088"
	resp := postJSON(t, server.URL+"/api/v1/auth/otp/request", map[string]string{
		"phoneNumber": phone, "countryCode": "+91",
	}, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	resp = postJSON(t, server.URL+"/api/v1/auth/otp/verify", map[string]interface{}{
		"phoneNumber": phone, "countryCode": "+91", "code": <-codes,
		"device": map[string]string{"platform": "android", "deviceName": "Deletion Test", "appVersion": "1.0.0"},
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var session authtransport.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&session))
	resp.Body.Close()

	resp = requestJSON(t, http.MethodGet, server.URL+"/api/v1/users/me/export", nil, session.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))
	require.Contains(t, resp.Header.Get("Cache-Control"), "no-store")
	exportBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()
	archive, err := zip.NewReader(bytes.NewReader(exportBytes), int64(len(exportBytes)))
	require.NoError(t, err)
	archiveFiles := make(map[string]string, len(archive.File))
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		archiveFiles[file.Name] = string(content)
	}
	require.Contains(t, archiveFiles, "metadata.json")
	require.Contains(t, archiveFiles, "profile.jsonl")
	require.Contains(t, archiveFiles, "sessions.jsonl")
	require.Contains(t, archiveFiles["profile.jsonl"], phone)
	for _, content := range archiveFiles {
		require.NotContains(t, content, session.RefreshToken)
		require.NotContains(t, content, "refreshTokenHash")
		require.NotContains(t, content, "pushToken")
	}

	resp = requestJSON(t, http.MethodDelete, server.URL+"/api/v1/users/me", map[string]string{
		"confirmation": "DELETE",
		"refreshToken": "invalid-refresh-credential-with-valid-length",
	}, session.AccessToken)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	resp = requestJSON(t, http.MethodGet, server.URL+"/api/v1/users/me", nil, session.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = requestJSON(t, http.MethodDelete, server.URL+"/api/v1/users/me", map[string]string{
		"confirmation": "DELETE",
		"refreshToken": session.RefreshToken,
	}, session.AccessToken)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	resp = requestJSON(t, http.MethodGet, server.URL+"/api/v1/users/me", nil, session.AccessToken)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
	resp = postJSON(t, server.URL+"/api/v1/auth/refresh", map[string]string{
		"refreshToken": session.RefreshToken,
	}, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}
