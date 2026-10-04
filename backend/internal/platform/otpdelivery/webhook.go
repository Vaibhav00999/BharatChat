package otpdelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxResponseBytes = 4 << 10

// Webhook sends OTPs to a deployment-owned HTTPS adapter. The adapter is the
// only component that needs provider-specific SMS credentials and payloads.
type Webhook struct {
	url         string
	bearerToken string
	client      *http.Client
}

func NewWebhook(url, bearerToken string, client *http.Client) *Webhook {
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Webhook{url: url, bearerToken: bearerToken, client: client}
}

func (w *Webhook) DeliverOTP(ctx context.Context, phoneNumber, code string) error {
	body, err := json.Marshal(map[string]string{
		"phoneNumber": phoneNumber,
		"code":        code,
		"purpose":     "login",
	})
	if err != nil {
		return fmt.Errorf("otp webhook: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("otp webhook: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.bearerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "bharatchat-otp/1")

	response, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("otp webhook: delivery failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("otp webhook: delivery returned HTTP %d", response.StatusCode)
	}
	return nil
}
