package otpdelivery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bharatchat/backend/internal/platform/otpdelivery"
	"github.com/stretchr/testify/require"
)

func TestWebhookDeliversAuthenticatedPayload(t *testing.T) {
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer webhook-secret", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	delivery := otpdelivery.NewWebhook(server.URL, "webhook-secret", server.Client())
	require.NoError(t, delivery.DeliverOTP(context.Background(), "+919876543210", "123456"))
	require.Equal(t, "+919876543210", payload["phoneNumber"])
	require.Equal(t, "123456", payload["code"])
	require.Equal(t, "login", payload["purpose"])
}

func TestWebhookRejectsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider internals must not leak", http.StatusBadGateway)
	}))
	defer server.Close()

	err := otpdelivery.NewWebhook(server.URL, "webhook-secret", server.Client()).
		DeliverOTP(context.Background(), "+919876543210", "123456")
	require.EqualError(t, err, "otp webhook: delivery returned HTTP 502")
}
