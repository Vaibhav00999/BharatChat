// backend/internal/features/auth/service/auth_service_test.go
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/bharatchat/backend/internal/features/auth/domain"
	"github.com/bharatchat/backend/internal/features/auth/service"
	"github.com/stretchr/testify/require"
)

// --- Test doubles (hand-written fakes; no mocking framework dependency needed) ---

type fakeOTPRepo struct {
	challenges map[string]*domain.OTPChallenge
	created    *domain.OTPChallenge
	lastCode   string
}

func newFakeOTPRepo() *fakeOTPRepo {
	return &fakeOTPRepo{challenges: map[string]*domain.OTPChallenge{}}
}

func (f *fakeOTPRepo) Create(ctx context.Context, phone, hash string, purpose domain.OTPPurpose, ttl time.Duration, maxAttempts int) (*domain.OTPChallenge, error) {
	c := &domain.OTPChallenge{ID: "otp-1", PhoneNumber: phone, OTPHash: hash, Purpose: purpose, MaxAttempts: maxAttempts, ExpiresAt: time.Now().Add(ttl)}
	f.challenges[phone] = c
	f.created = c
	return c, nil
}

func (f *fakeOTPRepo) FindLatestActive(ctx context.Context, phone string, purpose domain.OTPPurpose) (*domain.OTPChallenge, error) {
	c, ok := f.challenges[phone]
	if !ok || c.ConsumedAt != nil || time.Now().After(c.ExpiresAt) {
		return nil, nil
	}
	return c, nil
}

func (f *fakeOTPRepo) IncrementAttempt(ctx context.Context, id string) error {
	for _, c := range f.challenges {
		if c.ID == id {
			c.AttemptCount++
		}
	}
	return nil
}

func (f *fakeOTPRepo) MarkConsumed(ctx context.Context, id string) error {
	now := time.Now()
	for _, c := range f.challenges {
		if c.ID == id {
			c.ConsumedAt = &now
		}
	}
	return nil
}

type fakeOTPDelivery struct{ repo *fakeOTPRepo }

func (f fakeOTPDelivery) DeliverOTP(_ context.Context, _ string, code string) error {
	f.repo.lastCode = code
	return nil
}

type fakeSessionRepo struct {
	sessions map[string]*domain.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{sessions: map[string]*domain.Session{}}
}

func (f *fakeSessionRepo) Create(ctx context.Context, userID, deviceID, hash, ip, ua string, expiresAt time.Time) (*domain.Session, error) {
	s := &domain.Session{ID: "11111111-1111-4111-8111-111111111111", UserID: userID, DeviceID: deviceID, RefreshTokenHash: hash, ExpiresAt: expiresAt}
	f.sessions[hash] = s
	return s, nil
}

func (f *fakeSessionRepo) FindByRefreshTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	s, ok := f.sessions[hash]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (f *fakeSessionRepo) RotateRefreshToken(ctx context.Context, sessionID, newHash string, newExpiresAt time.Time) error {
	for _, s := range f.sessions {
		if s.ID == sessionID {
			delete(f.sessions, s.RefreshTokenHash)
			s.RefreshTokenHash = newHash
			s.ExpiresAt = newExpiresAt
			f.sessions[newHash] = s
		}
	}
	return nil
}

func (f *fakeSessionRepo) Revoke(ctx context.Context, sessionID string) error {
	now := time.Now()
	for _, s := range f.sessions {
		if s.ID == sessionID {
			s.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeSessionRepo) RevokeAllForUser(ctx context.Context, userID string) error { return nil }

type fakeDeviceRepo struct{}

func (f *fakeDeviceRepo) Upsert(ctx context.Context, userID string, info domain.DeviceInfo) (string, error) {
	return "device-1", nil
}

type fakeBlacklist struct{ revoked map[string]bool }

func newFakeBlacklist() *fakeBlacklist { return &fakeBlacklist{revoked: map[string]bool{}} }
func (f *fakeBlacklist) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	f.revoked[jti] = true
	return nil
}
func (f *fakeBlacklist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	return f.revoked[jti], nil
}

type allowAllRateLimiter struct{}

func (allowAllRateLimiter) Allow(ctx context.Context, phone string) (bool, error) { return true, nil }

type denyingRateLimiter struct{}

func (denyingRateLimiter) Allow(ctx context.Context, phone string) (bool, error) { return false, nil }

type fakeUserProvider struct {
	userID string
	isNew  bool
}

func (f *fakeUserProvider) FindOrCreateByPhone(ctx context.Context, phone, countryCode string) (string, bool, error) {
	return f.userID, f.isNew, nil
}

// realTokenService exercises the actual TokenService implementation for realistic
// JWT round-trip behavior in these otherwise-fake-repository-based unit tests.
func realTokenService() *service.TokenService {
	return service.NewTokenService("test-secret-key-for-unit-tests", 15*time.Minute)
}

func buildAuthService(t *testing.T, otpRepo *fakeOTPRepo, sessionRepo *fakeSessionRepo, blacklist *fakeBlacklist, rateLimiter domain.OTPRateLimiter, userProvider domain.UserProvider) *service.AuthService {
	t.Helper()
	tokenSvc := realTokenService()
	return service.NewAuthService(
		otpRepo, sessionRepo, &fakeDeviceRepo{}, blacklist, rateLimiter, userProvider,
		tokenSvc, tokenSvc,
		service.AuthServiceConfig{OTPTTL: 5 * time.Minute, OTPMaxAttempts: 5, RefreshTTLDays: 30},
		service.WithOTPDelivery(fakeOTPDelivery{repo: otpRepo}),
	)
}

func TestRequestOTP_Success(t *testing.T) {
	otpRepo := newFakeOTPRepo()
	svc := buildAuthService(t, otpRepo, newFakeSessionRepo(), newFakeBlacklist(), allowAllRateLimiter{}, &fakeUserProvider{})

	err := svc.RequestOTP(context.Background(), "+919876543210", "+91")
	require.NoError(t, err)
	require.NotNil(t, otpRepo.created)
	require.Equal(t, "+919876543210", otpRepo.created.PhoneNumber)
}

func TestRequestOTP_RateLimited(t *testing.T) {
	svc := buildAuthService(t, newFakeOTPRepo(), newFakeSessionRepo(), newFakeBlacklist(), denyingRateLimiter{}, &fakeUserProvider{})

	err := svc.RequestOTP(context.Background(), "+919876543210", "+91")
	require.Error(t, err)
}

func TestVerifyOTP_Success_NewUser(t *testing.T) {
	otpRepo := newFakeOTPRepo()
	sessionRepo := newFakeSessionRepo()
	userProvider := &fakeUserProvider{userID: "user-123", isNew: true}
	svc := buildAuthService(t, otpRepo, sessionRepo, newFakeBlacklist(), allowAllRateLimiter{}, userProvider)

	ctx := context.Background()
	require.NoError(t, svc.RequestOTP(ctx, "+919876543210", "+91"))

	// Extract the real code the fake repo stored the bcrypt hash for, by recomputing:
	// since RequestOTP hashes the code before storing it, tests need the actual code.
	// We recover it by re-hashing against known candidates is impractical; instead we
	// directly bcrypt-compare via a helper the fake exposes for test purposes only.
	rawCode := extractLastGeneratedCode(t, otpRepo)

	result, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{
		PhoneNumber: "+919876543210",
		CountryCode: "+91",
		Code:        rawCode,
		Device:      domain.DeviceInfo{Platform: domain.PlatformAndroid, DeviceName: "Pixel 8", AppVersion: "1.0.0"},
		IPAddress:   "127.0.0.1",
		UserAgent:   "test-agent",
	})

	require.NoError(t, err)
	require.True(t, result.IsNewUser)
	require.Equal(t, "user-123", result.UserID)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
}

func TestVerifyOTP_WrongCode_Fails(t *testing.T) {
	otpRepo := newFakeOTPRepo()
	svc := buildAuthService(t, otpRepo, newFakeSessionRepo(), newFakeBlacklist(), allowAllRateLimiter{}, &fakeUserProvider{userID: "user-1"})

	ctx := context.Background()
	require.NoError(t, svc.RequestOTP(ctx, "+919876543210", "+91"))

	_, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{
		PhoneNumber: "+919876543210",
		CountryCode: "+91",
		Code:        "000000",
		Device:      domain.DeviceInfo{Platform: domain.PlatformAndroid, DeviceName: "Pixel 8", AppVersion: "1.0.0"},
	})
	require.Error(t, err)
	require.Equal(t, 1, otpRepo.created.AttemptCount)
}

func TestRefreshToken_RotatesSuccessfully(t *testing.T) {
	otpRepo := newFakeOTPRepo()
	sessionRepo := newFakeSessionRepo()
	svc := buildAuthService(t, otpRepo, sessionRepo, newFakeBlacklist(), allowAllRateLimiter{}, &fakeUserProvider{userID: "user-1"})

	ctx := context.Background()
	require.NoError(t, svc.RequestOTP(ctx, "+919876543210", "+91"))
	rawCode := extractLastGeneratedCode(t, otpRepo)

	initial, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{
		PhoneNumber: "+919876543210", CountryCode: "+91", Code: rawCode,
		Device: domain.DeviceInfo{Platform: domain.PlatformAndroid, DeviceName: "Pixel 8", AppVersion: "1.0.0"},
	})
	require.NoError(t, err)

	refreshed, err := svc.RefreshToken(ctx, initial.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, initial.RefreshToken, refreshed.RefreshToken, "refresh token must rotate on use")
	require.NotEqual(t, initial.AccessToken, refreshed.AccessToken)

	// The old refresh token must no longer work after rotation.
	_, err = svc.RefreshToken(ctx, initial.RefreshToken)
	require.Error(t, err, "reusing a rotated-out refresh token must fail")
}

func TestAuthorizeAccountDeletion_RequiresCurrentSessionRefreshToken(t *testing.T) {
	otpRepo := newFakeOTPRepo()
	sessionRepo := newFakeSessionRepo()
	userID := "22222222-2222-4222-8222-222222222222"
	svc := buildAuthService(t, otpRepo, sessionRepo, newFakeBlacklist(), allowAllRateLimiter{}, &fakeUserProvider{userID: userID})

	ctx := context.Background()
	require.NoError(t, svc.RequestOTP(ctx, "+919876543210", "+91"))
	result, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{
		PhoneNumber: "+919876543210", CountryCode: "+91", Code: extractLastGeneratedCode(t, otpRepo),
		Device: domain.DeviceInfo{Platform: domain.PlatformAndroid, DeviceName: "Pixel 8", AppVersion: "1.0.0"},
	})
	require.NoError(t, err)

	claims, err := realTokenService().ParseAccessToken(result.AccessToken)
	require.NoError(t, err)
	sessionID := claims.SessionID.String()
	require.NoError(t, svc.AuthorizeAccountDeletion(ctx, userID, sessionID, result.RefreshToken))
	require.Error(t, svc.AuthorizeAccountDeletion(ctx, userID, sessionID, "wrong-refresh-token"))
	require.Error(t, svc.AuthorizeAccountDeletion(ctx, "33333333-3333-4333-8333-333333333333", sessionID, result.RefreshToken))
	require.Error(t, svc.AuthorizeAccountDeletion(ctx, userID, "44444444-4444-4444-8444-444444444444", result.RefreshToken))

	hash := realTokenService().HashRefreshToken(result.RefreshToken)
	session := sessionRepo.sessions[hash]
	now := time.Now()
	session.RevokedAt = &now
	require.Error(t, svc.AuthorizeAccountDeletion(ctx, userID, sessionID, result.RefreshToken))
	session.RevokedAt = nil
	session.ExpiresAt = now.Add(-time.Second)
	require.Error(t, svc.AuthorizeAccountDeletion(ctx, userID, sessionID, result.RefreshToken))
}

func TestLogout_RevokesSessionAndBlacklistsToken(t *testing.T) {
	otpRepo := newFakeOTPRepo()
	sessionRepo := newFakeSessionRepo()
	blacklist := newFakeBlacklist()
	svc := buildAuthService(t, otpRepo, sessionRepo, blacklist, allowAllRateLimiter{}, &fakeUserProvider{userID: "22222222-2222-4222-8222-222222222222"})

	ctx := context.Background()
	require.NoError(t, svc.RequestOTP(ctx, "+919876543210", "+91"))
	rawCode := extractLastGeneratedCode(t, otpRepo)

	result, err := svc.VerifyOTP(ctx, service.VerifyOTPInput{
		PhoneNumber: "+919876543210", CountryCode: "+91", Code: rawCode,
		Device: domain.DeviceInfo{Platform: domain.PlatformAndroid, DeviceName: "Pixel 8", AppVersion: "1.0.0"},
	})
	require.NoError(t, err)

	tokenSvc := realTokenService()
	claims, err := tokenSvc.ParseAccessToken(result.AccessToken)
	require.NoError(t, err)

	err = svc.Logout(ctx, claims.SessionID.String(), claims.JTI, 15*time.Minute)
	require.NoError(t, err)

	revoked, _ := blacklist.IsRevoked(ctx, claims.JTI)
	require.True(t, revoked)

	_, err = svc.RefreshToken(ctx, result.RefreshToken)
	require.Error(t, err, "refresh token must not work after logout revokes the session")
}

func extractLastGeneratedCode(t *testing.T, repo *fakeOTPRepo) string {
	t.Helper()
	require.NotEmpty(t, repo.lastCode, "expected fake delivery to capture an OTP")
	return repo.lastCode
}
