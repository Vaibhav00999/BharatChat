package otpdelivery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMSG91Request(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		q := r.URL.Query()
		require.Equal(t, "919876543210", q.Get("mobile"))
		require.Equal(t, "012345", q.Get("otp"))
		require.Equal(t, "5", q.Get("otp_expiry"))
		require.Equal(t, "6", q.Get("otp_length"))
		require.Equal(t, "secret-key", q.Get("authkey"))
		require.Equal(t, "approved-template", q.Get("template_id"))
		_, _ = w.Write([]byte(`{"type":"success","request_id":"accepted"}`))
	}))
	defer srv.Close()
	m := NewMSG91("secret-key", "approved-template", 5*time.Minute, nil)
	m.endpoint = srv.URL
	require.NoError(t, m.DeliverOTP(context.Background(), "+919876543210", "012345"))
}

func TestMSG91RejectsFailuresWithoutLeakingResponse(t *testing.T) {
	for _, body := range []string{`{"type":"error","message":"sensitive"}`, `{"type":"success"}`, `not-json-sensitive`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			m := NewMSG91("secret-key", "template", time.Minute, nil)
			m.endpoint = srv.URL
			err := m.DeliverOTP(context.Background(), "+919876543210", "012345")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "sensitive")
		})
	}
}

type failingMSG91Transport struct{}

func (failingMSG91Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New(r.URL.String())
}

func TestMSG91RedactsNetworkErrorsAndRejectsRedirects(t *testing.T) {
	m := NewMSG91("secret-key", "template", time.Minute, &http.Client{Transport: failingMSG91Transport{}})
	err := m.DeliverOTP(context.Background(), "+919876543210", "012345")
	require.EqualError(t, err, "msg91 otp: delivery request failed")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	m = NewMSG91("secret-key", "template", time.Minute, nil)
	m.endpoint = srv.URL
	require.EqualError(t, m.DeliverOTP(context.Background(), "+919876543210", "012345"), "msg91 otp: provider returned HTTP 307")
}
