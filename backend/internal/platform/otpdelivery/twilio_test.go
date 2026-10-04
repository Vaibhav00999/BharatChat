package otpdelivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTwilioMessagingDeliversAuthenticatedForm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2010-04-01/Accounts/ACaccount/Messages.json", r.URL.Path)
		username, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "SKkey", username)
		require.Equal(t, "api-secret", password)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "+919876543210", r.Form.Get("To"))
		require.Equal(t, "MGservice", r.Form.Get("MessagingServiceSid"))
		require.Equal(t, "BharatChat code 123456 expires in 5 minutes.", r.Form.Get("Body"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
			"sid": "SMmessage", "status": "queued",
		}))
	}))
	defer server.Close()

	delivery := newTwilioMessaging(
		"ACaccount", "SKkey", "api-secret", "MGservice",
		"BharatChat code {{CODE}} expires in {{MINUTES}} minutes.",
		5*time.Minute, server.Client(), server.URL,
	)
	require.NoError(t, delivery.DeliverOTP(context.Background(), "+919876543210", "123456"))
}

func TestTwilioMessagingRejectsProviderFailureWithoutLeakingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "credential and provider details", http.StatusUnauthorized)
	}))
	defer server.Close()

	delivery := newTwilioMessaging(
		"ACaccount", "SKkey", "api-secret", "MGservice", "Code {{CODE}}",
		5*time.Minute, server.Client(), server.URL,
	)
	require.EqualError(t,
		delivery.DeliverOTP(context.Background(), "+919876543210", "123456"),
		"twilio otp: delivery returned HTTP 401",
	)
}

func TestTwilioMessagingRejectsMalformedSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	defer server.Close()

	delivery := newTwilioMessaging(
		"ACaccount", "SKkey", "api-secret", "MGservice", "Code {{CODE}}",
		5*time.Minute, server.Client(), server.URL,
	)
	require.EqualError(t,
		delivery.DeliverOTP(context.Background(), "+919876543210", "123456"),
		"twilio otp: provider did not accept the message",
	)
}
