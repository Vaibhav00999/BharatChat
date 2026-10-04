package service_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bharatchat/backend/internal/features/auth/service"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func TestPilotDeniesUninvitedNumbersBeforeAnySideEffects(t *testing.T) {
	for _, phones := range [][]string{nil, {}, {"+12025550100"}} {
		// Nil dependencies deliberately fail the test if any downstream work occurs.
		svc := service.NewAuthService(nil, nil, nil, nil, nil, nil, nil, nil,
			service.AuthServiceConfig{}, service.WithAllowedPhones(phones))
		ctx := context.Background()
		err := svc.RequestOTP(ctx, "+12025550101", "+1")
		appErr, ok := apperror.As(err)
		require.True(t, ok)
		require.Equal(t, http.StatusForbidden, appErr.HTTPStatus)
		result, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{PhoneNumber: "+12025550101", Code: "123456"})
		require.Nil(t, result)
		appErr, ok = apperror.As(err)
		require.True(t, ok)
		require.Equal(t, http.StatusForbidden, appErr.HTTPStatus)
	}
}

func TestPilotAllowedNumberCanRequestAndVerify(t *testing.T) {
	repo := newFakeOTPRepo()
	tokens := realTokenService()
	phones := []string{"+12025550100"}
	svc := service.NewAuthService(repo, newFakeSessionRepo(), &fakeDeviceRepo{}, newFakeBlacklist(),
		allowAllRateLimiter{}, &fakeUserProvider{userID: "pilot-user"}, tokens, tokens,
		service.AuthServiceConfig{OTPTTL: time.Minute, OTPMaxAttempts: 5, RefreshTTLDays: 1},
		service.WithAllowedPhones(phones), service.WithOTPDelivery(fakeOTPDelivery{repo: repo}))
	phones[0] = "+12025550101" // The option must not retain the caller's mutable slice.
	ctx := context.Background()
	require.NoError(t, svc.RequestOTP(ctx, "+12025550100", "+1"))
	require.NotEmpty(t, repo.lastCode)
	result, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{PhoneNumber: "+12025550100", Code: repo.lastCode})
	require.NoError(t, err)
	require.NotEmpty(t, result.AccessToken)
}
