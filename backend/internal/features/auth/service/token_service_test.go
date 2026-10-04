package service_test

import (
	"testing"
	"time"

	"github.com/bharatchat/backend/internal/features/auth/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTokenServiceValidatesIssuerAndAudience(t *testing.T) {
	userID, sessionID := uuid.NewString(), uuid.NewString()
	issuer := service.NewTokenService("0123456789abcdef0123456789abcdef", time.Minute, "issuer-a", "audience-a")
	token, _, _, err := issuer.GenerateAccessToken(userID, sessionID)
	require.NoError(t, err)

	claims, err := issuer.ParseAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, userID, claims.UserID.String())
	require.Equal(t, sessionID, claims.SessionID.String())

	wrongIssuer := service.NewTokenService("0123456789abcdef0123456789abcdef", time.Minute, "issuer-b", "audience-a")
	_, err = wrongIssuer.ParseAccessToken(token)
	require.Error(t, err)

	wrongAudience := service.NewTokenService("0123456789abcdef0123456789abcdef", time.Minute, "issuer-a", "audience-b")
	_, err = wrongAudience.ParseAccessToken(token)
	require.Error(t, err)
}

func TestTokenServiceRejectsDifferentSigningSecret(t *testing.T) {
	issuer := service.NewTokenService("0123456789abcdef0123456789abcdef", time.Minute)
	token, _, _, err := issuer.GenerateAccessToken(uuid.NewString(), uuid.NewString())
	require.NoError(t, err)

	verifier := service.NewTokenService("abcdef0123456789abcdef0123456789", time.Minute)
	_, err = verifier.ParseAccessToken(token)
	require.Error(t, err)
}
