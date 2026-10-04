package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMSG91Configuration(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_ACCESS_SECRET", "test-secret")
	t.Setenv("OTP_DELIVERY_MODE", "msg91")
	t.Setenv("OTP_MSG91_AUTH_KEY", "")
	t.Setenv("OTP_MSG91_TEMPLATE_ID", "")
	_, err := Load()
	require.ErrorContains(t, err, "OTP_MSG91_AUTH_KEY")
	t.Setenv("OTP_MSG91_AUTH_KEY", "test-key-not-a-real-provider-key")
	_, err = Load()
	require.ErrorContains(t, err, "OTP_MSG91_TEMPLATE_ID")
	t.Setenv("OTP_MSG91_TEMPLATE_ID", "0123456789abcdef01234567")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "msg91", cfg.OTP.DeliveryMode)
}
