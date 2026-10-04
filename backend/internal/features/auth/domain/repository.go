package domain

import (
	"context"
	"errors"
	"time"
)

var ErrOTPChallengeInactive = errors.New("OTP challenge is no longer active")

type OTPRepository interface {
	Create(context.Context, string, string, OTPPurpose, time.Duration, int) (*OTPChallenge, error)
	FindLatestActive(context.Context, string, OTPPurpose) (*OTPChallenge, error)
	IncrementAttempt(context.Context, string) error
	MarkConsumed(context.Context, string) error
}
type DeviceRepository interface {
	Upsert(context.Context, string, DeviceInfo) (string, error)
}
type SessionRepository interface {
	Create(context.Context, string, string, string, string, string, time.Time) (*Session, error)
	FindByRefreshTokenHash(context.Context, string) (*Session, error)
	RotateRefreshToken(context.Context, string, string, time.Time) error
	Revoke(context.Context, string) error
	RevokeAllForUser(context.Context, string) error
}
type UserProvider interface {
	FindOrCreateByPhone(context.Context, string, string) (string, bool, error)
}
type OTPRateLimiter interface {
	Allow(context.Context, string) (bool, error)
}
type TokenBlacklist interface {
	Revoke(context.Context, string, time.Duration) error
	IsRevoked(context.Context, string) (bool, error)
}
