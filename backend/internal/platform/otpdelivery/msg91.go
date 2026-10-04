package otpdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MSG91 delivers our challenge through SendOTP v5; verification stays local.
type MSG91 struct {
	authKey, templateID string
	ttl                 time.Duration
	client              *http.Client
	endpoint            string
}

func NewMSG91(authKey, templateID string, ttl time.Duration, client *http.Client) *MSG91 {
	c := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		c = *client
	}
	c.Timeout = 10 * time.Second
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &MSG91{authKey: authKey, templateID: templateID, ttl: ttl, client: &c, endpoint: "https://control.msg91.com/api/v5/otp"}
}

func (m *MSG91) DeliverOTP(ctx context.Context, phoneNumber, code string) error {
	query := url.Values{
		"authkey": {m.authKey}, "template_id": {m.templateID},
		"mobile": {strings.TrimPrefix(phoneNumber, "+")}, "otp": {code},
		"otp_length": {"6"}, "otp_expiry": {strconv.Itoa(int(m.ttl.Minutes()))},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint+"?"+query.Encode(), strings.NewReader("{}"))
	if err != nil {
		return errors.New("msg91 otp: invalid request configuration")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	// The documented API places credentials and the OTP in the query string.
	// Never wrap HTTP errors (which contain that URL) or return provider bodies.
	if err != nil {
		return errors.New("msg91 otp: delivery request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("msg91 otp: provider returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&result) != nil {
		return errors.New("msg91 otp: invalid provider response")
	}
	if result.Type != "success" || result.RequestID == "" {
		return errors.New("msg91 otp: provider did not accept the message")
	}
	return nil
}
