package otpdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const twilioAPIBaseURL = "https://api.twilio.com"

// TwilioMessaging sends a caller-generated OTP through Twilio Programmable
// Messaging. API-key credentials are used instead of the account auth token so
// this workload can be assigned and rotated independently.
type TwilioMessaging struct {
	accountSID          string
	apiKey              string
	apiSecret           string
	messagingServiceSID string
	messageTemplate     string
	codeTTL             time.Duration
	client              *http.Client
	baseURL             string
}

func NewTwilioMessaging(
	accountSID, apiKey, apiSecret, messagingServiceSID, messageTemplate string,
	codeTTL time.Duration,
	client *http.Client,
) *TwilioMessaging {
	return newTwilioMessaging(
		accountSID, apiKey, apiSecret, messagingServiceSID, messageTemplate,
		codeTTL, client, twilioAPIBaseURL,
	)
}

func newTwilioMessaging(
	accountSID, apiKey, apiSecret, messagingServiceSID, messageTemplate string,
	codeTTL time.Duration,
	client *http.Client,
	baseURL string,
) *TwilioMessaging {
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &TwilioMessaging{
		accountSID: accountSID, apiKey: apiKey, apiSecret: apiSecret,
		messagingServiceSID: messagingServiceSID, messageTemplate: messageTemplate,
		codeTTL: codeTTL, client: client, baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (t *TwilioMessaging) DeliverOTP(ctx context.Context, phoneNumber, code string) error {
	message := strings.Replace(t.messageTemplate, "{{CODE}}", code, 1)
	message = strings.ReplaceAll(message, "{{MINUTES}}", fmt.Sprintf("%d", int(t.codeTTL.Minutes())))

	form := url.Values{}
	form.Set("To", phoneNumber)
	form.Set("MessagingServiceSid", t.messagingServiceSID)
	form.Set("Body", message)

	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", t.baseURL, t.accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("twilio otp: create request: %w", err)
	}
	req.SetBasicAuth(t.apiKey, t.apiSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "bharatchat-otp/1")

	response, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio otp: delivery failed: %w", err)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, maxResponseBytes)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, limited)
		return fmt.Errorf("twilio otp: delivery returned HTTP %d", response.StatusCode)
	}

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(limited).Decode(&result); err != nil {
		return fmt.Errorf("twilio otp: decode response: %w", err)
	}
	if result.SID == "" || result.Status == "failed" || result.Status == "undelivered" {
		return fmt.Errorf("twilio otp: provider did not accept the message")
	}
	return nil
}
