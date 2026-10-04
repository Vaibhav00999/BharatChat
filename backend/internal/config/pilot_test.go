package config

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPilotPhoneAllowlistValidation(t *testing.T) {
	phones := make([]string, 21)
	for i := range phones {
		phones[i] = fmt.Sprintf("+1202555%04d", i)
	}
	for _, raw := range []string{"*", " ", "+12025550100,", "+12025550100, +12025550101", "+12025550100,+12025550100", "12025550100", "+01234567890", "+12025550100\n", strings.Join(phones, ",")} {
		t.Run(fmt.Sprintf("invalid-%d", len(raw))+raw[:1], func(t *testing.T) {
			_, err := parseAllowedPhones(raw)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "+12025550100")
		})
	}
	allowed, err := parseAllowedPhones(strings.Join(phones[:20], ","))
	require.NoError(t, err)
	require.Len(t, allowed, 20)
}

func pilotEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_ACCESS_SECRET", strings.Repeat("x", 48))
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("POSTGRES_PASSWORD", "test-db-password")
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_PASSWORD", "test-cache-password")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://bharatchat.in")
	t.Setenv("REQUIRE_E2EE", "true")
	t.Setenv("OTP_DELIVERY_MODE", "msg91")
	t.Setenv("OTP_MSG91_AUTH_KEY", "test-only-provider-key")
	t.Setenv("OTP_MSG91_TEMPLATE_ID", "0123456789abcdef")
	t.Setenv("OTP_ALLOWED_PHONES", "+12025550100")
	t.Setenv("MESSAGING_ENABLED", "false")
}

func TestPilotProductionRequiresTestNumbers(t *testing.T) {
	pilotEnvironment(t)
	t.Setenv("OTP_ALLOWED_PHONES", "")
	_, err := Load()
	require.ErrorContains(t, err, "OTP_ALLOWED_PHONES is required")
}

func TestPilotProductionDisablesMessaging(t *testing.T) {
	pilotEnvironment(t)
	// t.Setenv above preserves the original environment for cleanup.
	require.NoError(t, os.Unsetenv("MESSAGING_ENABLED"))
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.MessagingEnabled)
	require.Equal(t, []string{"+12025550100"}, cfg.OTP.AllowedPhones)
	t.Setenv("MESSAGING_ENABLED", "true")
	_, err = Load()
	require.ErrorContains(t, err, "E2EE launch gate")
}

func TestPilotRejectsMalformedMessagingSwitch(t *testing.T) {
	pilotEnvironment(t)
	for _, value := range []string{"", "yes", "TRUE", "false ", "0"} {
		t.Setenv("MESSAGING_ENABLED", value)
		_, err := Load()
		require.ErrorContains(t, err, "MESSAGING_ENABLED")
	}
}

func TestPilotPreservesLocalDevelopment(t *testing.T) {
	pilotEnvironment(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("OTP_ALLOWED_PHONES", "")
	require.NoError(t, os.Unsetenv("MESSAGING_ENABLED"))
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.MessagingEnabled)
	require.Empty(t, cfg.OTP.AllowedPhones)
}
