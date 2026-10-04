package config

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Deliberately public test fixture, never a deployment credential.
const testCredential = "0123456789abcdef0123456789abcdef" // gitleaks:allow

func TestPostgresDSNRoundTripsSpecialCharacters(t *testing.T) {
	dsn := PostgresConfig{
		Host:     "2001:db8::1",
		Port:     "5432",
		User:     "chat user",
		Password: "p@ss:/?#word",
		DBName:   "bharatchat",
		SSLMode:  "require",
	}.DSN()

	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "chat user", parsed.User.Username())
	password, ok := parsed.User.Password()
	require.True(t, ok)
	require.Equal(t, "p@ss:/?#word", password)
	require.Equal(t, "[2001:db8::1]:5432", parsed.Host)
	require.Equal(t, "require", parsed.Query().Get("sslmode"))
}

func TestLoadRejectsUnsafeProductionDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", "change_this_access_secret_in_prod")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	_, err := Load()
	require.Error(t, err)
}

func TestLoadAcceptsExplicitProductionSecuritySettings(t *testing.T) {
	t.Setenv("OTP_ALLOWED_PHONES", "+12025550100")
	t.Setenv("MESSAGING_ENABLED", "false")
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", testCredential)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://chat.example.com")
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("POSTGRES_PASSWORD", "production-postgres-secret")
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_PASSWORD", "production-redis-secret")
	t.Setenv("OTP_DELIVERY_MODE", "webhook")
	t.Setenv("OTP_WEBHOOK_URL", "https://sms.example.com/v1/otp")
	t.Setenv("OTP_WEBHOOK_BEARER_TOKEN", testCredential)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []string{"https://chat.example.com"}, cfg.CORSAllowedOrigins)
	require.True(t, cfg.Privacy.RequireE2EE)
}

func TestLoadRejectsConsoleOTPDeliveryInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", testCredential)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://chat.example.com")
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("POSTGRES_PASSWORD", "production-postgres-secret")
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_PASSWORD", "production-redis-secret")
	t.Setenv("OTP_DELIVERY_MODE", "console")

	_, err := Load()
	require.ErrorContains(t, err, "OTP_DELIVERY_MODE")
}

func TestLoadRejectsInsecureOTPWebhook(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_ACCESS_SECRET", "development-secret")
	t.Setenv("OTP_DELIVERY_MODE", "webhook")
	t.Setenv("OTP_WEBHOOK_URL", "http://sms.example.com/v1/otp")
	t.Setenv("OTP_WEBHOOK_BEARER_TOKEN", testCredential)

	_, err := Load()
	require.ErrorContains(t, err, "absolute HTTPS URL")
}

func TestLoadRejectsDisablingE2EEInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", testCredential)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://chat.example.com")
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REQUIRE_E2EE", "false")

	_, err := Load()
	require.Error(t, err)
}

func TestLoadRejectsPlaintextProductionDatastores(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", testCredential)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://chat.example.com")
	t.Setenv("POSTGRES_SSLMODE", "disable")
	t.Setenv("REDIS_TLS", "false")

	_, err := Load()
	require.Error(t, err)
}

func TestLoadAcceptsProductionTwilioOTPDelivery(t *testing.T) {
	t.Setenv("OTP_ALLOWED_PHONES", "+12025550100")
	t.Setenv("MESSAGING_ENABLED", "false")
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", testCredential)
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://chat.example.com")
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("POSTGRES_PASSWORD", "production-postgres-secret")
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_PASSWORD", "production-redis-secret")
	t.Setenv("OTP_DELIVERY_MODE", "twilio")
	t.Setenv("OTP_TWILIO_ACCOUNT_SID", "AC"+strings.Repeat("a", 32))
	t.Setenv("OTP_TWILIO_API_KEY", "SK"+strings.Repeat("b", 32))
	t.Setenv("OTP_TWILIO_API_SECRET", strings.Repeat("c", 32))
	t.Setenv("OTP_TWILIO_MESSAGING_SERVICE_SID", "MG"+strings.Repeat("d", 32))
	t.Setenv("OTP_SMS_MESSAGE_TEMPLATE", "Your BharatChat code is {{CODE}}. It expires in {{MINUTES}} minutes.")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "twilio", cfg.OTP.DeliveryMode)
	require.Equal(t, "AC"+strings.Repeat("a", 32), cfg.OTP.TwilioAccountSID)
}

func TestLoadRejectsTwilioTemplateWithoutCodePlaceholder(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_ACCESS_SECRET", "development-secret")
	t.Setenv("OTP_DELIVERY_MODE", "twilio")
	t.Setenv("OTP_TWILIO_ACCOUNT_SID", "AC"+strings.Repeat("a", 32))
	t.Setenv("OTP_TWILIO_API_KEY", "SK"+strings.Repeat("b", 32))
	t.Setenv("OTP_TWILIO_API_SECRET", strings.Repeat("c", 32))
	t.Setenv("OTP_TWILIO_MESSAGING_SERVICE_SID", "MG"+strings.Repeat("d", 32))
	t.Setenv("OTP_SMS_MESSAGE_TEMPLATE", "Your BharatChat verification code is missing.")

	_, err := Load()
	require.ErrorContains(t, err, "must contain {{CODE}} exactly once")
}
