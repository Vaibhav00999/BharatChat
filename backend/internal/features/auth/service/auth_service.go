// internal/features/auth/service/auth_service.go
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/bharatchat/backend/internal/features/auth/domain"
	"github.com/bharatchat/backend/pkg/apperror"
	"golang.org/x/crypto/bcrypt"
)

// AccessTokenIssuer + RefreshTokenIssuer split TokenService's surface into the two
// capabilities AuthService actually needs (Interface Segregation), while
// middleware.TokenParser covers the third (verification, used only by middleware).
type AccessTokenIssuer interface {
	GenerateAccessToken(userID, sessionID string) (token, jti string, expiresAt time.Time, err error)
}

type RefreshTokenIssuer interface {
	GenerateRefreshToken() (raw, hash string, err error)
	HashRefreshToken(raw string) string
}

type refreshTokenCASRepository interface {
	RotateRefreshTokenIfCurrent(ctx context.Context, sessionID, currentHash, newHash string, newExpiresAt time.Time) (bool, error)
}

// OTPDelivery is the boundary for an SMS provider. Production must explicitly
// configure an implementation; raw codes are never exposed by AuthService.
type OTPDelivery interface {
	DeliverOTP(ctx context.Context, phoneNumber, code string) error
}

type OTPDeliveryFunc func(ctx context.Context, phoneNumber, code string) error

func (f OTPDeliveryFunc) DeliverOTP(ctx context.Context, phoneNumber, code string) error {
	return f(ctx, phoneNumber, code)
}

type AuthServiceOption func(*AuthService)

func WithOTPDelivery(delivery OTPDelivery) AuthServiceOption {
	return func(service *AuthService) { service.otpDelivery = delivery }
}

// WithAllowedPhones restricts new logins as well as SMS sends. An explicitly empty
// list denies all numbers; omitting this option retains unrestricted local dev.
func WithAllowedPhones(phones []string) AuthServiceOption {
	return func(service *AuthService) {
		service.allowedPhones = make(map[string]struct{}, len(phones))
		for _, phone := range phones {
			service.allowedPhones[phone] = struct{}{}
		}
	}
}

type AuthService struct {
	otpRepo       domain.OTPRepository
	sessionRepo   domain.SessionRepository
	deviceRepo    domain.DeviceRepository
	blacklist     domain.TokenBlacklist
	rateLimiter   domain.OTPRateLimiter
	userProvider  domain.UserProvider
	accessIssuer  AccessTokenIssuer
	refreshIssuer RefreshTokenIssuer
	otpDelivery   OTPDelivery
	allowedPhones map[string]struct{}

	otpTTL         time.Duration
	otpMaxAttempts int
	refreshTTLDays int
}

type AuthServiceConfig struct {
	OTPTTL         time.Duration
	OTPMaxAttempts int
	RefreshTTLDays int
}

func NewAuthService(
	otpRepo domain.OTPRepository,
	sessionRepo domain.SessionRepository,
	deviceRepo domain.DeviceRepository,
	blacklist domain.TokenBlacklist,
	rateLimiter domain.OTPRateLimiter,
	userProvider domain.UserProvider,
	accessIssuer AccessTokenIssuer,
	refreshIssuer RefreshTokenIssuer,
	cfg AuthServiceConfig,
	options ...AuthServiceOption,
) *AuthService {
	service := &AuthService{
		otpRepo:        otpRepo,
		sessionRepo:    sessionRepo,
		deviceRepo:     deviceRepo,
		blacklist:      blacklist,
		rateLimiter:    rateLimiter,
		userProvider:   userProvider,
		accessIssuer:   accessIssuer,
		refreshIssuer:  refreshIssuer,
		otpTTL:         cfg.OTPTTL,
		otpMaxAttempts: cfg.OTPMaxAttempts,
		refreshTTLDays: cfg.RefreshTTLDays,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// generateOTP returns a zero-padded 6-digit numeric code using crypto/rand — never
// math/rand, since OTPs are a security boundary.
func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// RequestOTP generates and delivers a one-time code through the configured provider
// after checking the per-phone-number rate limit.
func (s *AuthService) RequestOTP(ctx context.Context, phoneNumber, countryCode string) error {
	if err := s.authorizePhone(phoneNumber); err != nil {
		return err
	}
	allowed, err := s.rateLimiter.Allow(ctx, phoneNumber)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !allowed {
		return apperror.NewTooManyRequests("too many OTP requests for this number, please try again later")
	}

	code, err := generateOTP()
	if err != nil {
		return apperror.NewInternal(err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return apperror.NewInternal(err)
	}

	if _, err := s.otpRepo.Create(ctx, phoneNumber, string(hash), domain.OTPPurposeLogin, s.otpTTL, s.otpMaxAttempts); err != nil {
		return apperror.NewInternal(err)
	}

	if s.otpDelivery == nil {
		return apperror.NewInternal(errors.New("OTP delivery provider is not configured"))
	}
	if err := s.otpDelivery.DeliverOTP(ctx, phoneNumber, code); err != nil {
		return apperror.NewInternal(err)
	}

	return nil
}

type VerifyOTPInput struct {
	PhoneNumber string
	CountryCode string
	Code        string
	Device      domain.DeviceInfo
	IPAddress   string
	UserAgent   string
}

func (s *AuthService) VerifyOTP(ctx context.Context, input VerifyOTPInput) (*domain.AuthResult, error) {
	if err := s.authorizePhone(input.PhoneNumber); err != nil {
		return nil, err
	}
	challenge, err := s.otpRepo.FindLatestActive(ctx, input.PhoneNumber, domain.OTPPurposeLogin)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if challenge == nil {
		return nil, apperror.NewValidation("no active OTP found for this number, please request a new one")
	}
	if challenge.AttemptCount >= challenge.MaxAttempts {
		return nil, apperror.NewTooManyRequests("maximum verification attempts exceeded, please request a new OTP")
	}

	if bcryptErr := bcrypt.CompareHashAndPassword([]byte(challenge.OTPHash), []byte(input.Code)); bcryptErr != nil {
		if incErr := s.otpRepo.IncrementAttempt(ctx, challenge.ID); incErr != nil {
			return nil, apperror.NewInternal(incErr)
		}
		return nil, apperror.NewValidation("incorrect OTP code")
	}

	if err := s.otpRepo.MarkConsumed(ctx, challenge.ID); err != nil {
		if errors.Is(err, domain.ErrOTPChallengeInactive) {
			return nil, apperror.NewValidation("OTP has already been used or expired")
		}
		return nil, apperror.NewInternal(err)
	}

	userID, isNew, err := s.userProvider.FindOrCreateByPhone(ctx, input.PhoneNumber, input.CountryCode)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	deviceID, err := s.deviceRepo.Upsert(ctx, userID, input.Device)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	result, err := s.issueNewSession(ctx, userID, deviceID, input.IPAddress, input.UserAgent)
	if err != nil {
		return nil, err
	}
	result.IsNewUser = isNew

	return result, nil
}

// issueNewSession creates a fresh session row + refresh/access token pair. Shared by
// VerifyOTP (brand-new session) — RefreshToken has its own rotation path below since
// it updates an existing session rather than creating one.
func (s *AuthService) issueNewSession(ctx context.Context, userID, deviceID, ipAddress, userAgent string) (*domain.AuthResult, error) {
	refreshRaw, refreshHash, err := s.refreshIssuer.GenerateRefreshToken()
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	expiresAt := time.Now().AddDate(0, 0, s.refreshTTLDays)

	session, err := s.sessionRepo.Create(ctx, userID, deviceID, refreshHash, ipAddress, userAgent, expiresAt)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	accessToken, _, accessExpiresAt, err := s.accessIssuer.GenerateAccessToken(userID, session.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	return &domain.AuthResult{
		UserID:               userID,
		AccessToken:          accessToken,
		RefreshToken:         refreshRaw,
		AccessTokenExpiresAt: accessExpiresAt,
	}, nil
}

// RefreshToken rotates a refresh token: validates the presented raw token's hash
// against an active, non-expired, non-revoked session, then issues a brand-new
// refresh token + access token, invalidating the old refresh token immediately.
func (s *AuthService) RefreshToken(ctx context.Context, rawRefreshToken string) (*domain.AuthResult, error) {
	hash := s.refreshIssuer.HashRefreshToken(rawRefreshToken)

	session, err := s.sessionRepo.FindByRefreshTokenHash(ctx, hash)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if session == nil {
		return nil, apperror.NewUnauthorized("invalid refresh token")
	}
	if session.RevokedAt != nil {
		return nil, apperror.NewUnauthorized("session has been revoked")
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, apperror.NewUnauthorized("refresh token has expired, please log in again")
	}

	newRefreshRaw, newRefreshHash, err := s.refreshIssuer.GenerateRefreshToken()
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	newExpiresAt := time.Now().AddDate(0, 0, s.refreshTTLDays)

	if casRepo, ok := s.sessionRepo.(refreshTokenCASRepository); ok {
		rotated, err := casRepo.RotateRefreshTokenIfCurrent(ctx, session.ID, hash, newRefreshHash, newExpiresAt)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if !rotated {
			return nil, apperror.NewUnauthorized("refresh token has already been used")
		}
	} else if err := s.sessionRepo.RotateRefreshToken(ctx, session.ID, newRefreshHash, newExpiresAt); err != nil {
		return nil, apperror.NewInternal(err)
	}

	accessToken, _, accessExpiresAt, err := s.accessIssuer.GenerateAccessToken(session.UserID, session.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	return &domain.AuthResult{
		UserID:               session.UserID,
		AccessToken:          accessToken,
		RefreshToken:         newRefreshRaw,
		AccessTokenExpiresAt: accessExpiresAt,
	}, nil
}

// AuthorizeAccountDeletion provides step-up authorization for the destructive
// account-erasure operation. A bearer access token alone is insufficient: the
// caller must also prove possession of the current refresh credential for the
// exact authenticated session.
func (s *AuthService) AuthorizeAccountDeletion(ctx context.Context, userID, sessionID, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return apperror.NewUnauthorized("current session confirmation is required")
	}

	hash := s.refreshIssuer.HashRefreshToken(rawRefreshToken)
	session, err := s.sessionRepo.FindByRefreshTokenHash(ctx, hash)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if session == nil || session.ID != sessionID || session.UserID != userID {
		return apperror.NewUnauthorized("current session confirmation is invalid")
	}
	if session.RevokedAt != nil || !time.Now().Before(session.ExpiresAt) {
		return apperror.NewUnauthorized("current session has expired or been revoked")
	}

	return nil
}

// Logout revokes the session (invalidating its refresh token permanently) and
// blacklists the current access token's JTI so it stops working immediately
// rather than merely expiring naturally over the following minutes.
func (s *AuthService) Logout(ctx context.Context, sessionID, currentAccessTokenJTI string, accessTokenRemainingTTL time.Duration) error {
	if err := s.sessionRepo.Revoke(ctx, sessionID); err != nil {
		return apperror.NewInternal(err)
	}
	if currentAccessTokenJTI != "" {
		if err := s.blacklist.Revoke(ctx, currentAccessTokenJTI, accessTokenRemainingTTL); err != nil {
			return apperror.NewInternal(err)
		}
	}
	return nil
}

var ErrSessionNotFound = errors.New("session not found")

func (s *AuthService) authorizePhone(phone string) error {
	if s.allowedPhones == nil {
		return nil
	}
	if _, allowed := s.allowedPhones[phone]; !allowed {
		return apperror.NewForbidden("BharatChat is currently available to invited testers only")
	}
	return nil
}
