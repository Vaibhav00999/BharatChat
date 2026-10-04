package domain

import "time"

type OTPPurpose string

const (
	OTPPurposeLogin        OTPPurpose = "login"
	OTPPurposeChangeNumber OTPPurpose = "change_number"
)

type OTPChallenge struct {
	ID, PhoneNumber, OTPHash  string
	Purpose                   OTPPurpose
	AttemptCount, MaxAttempts int
	ExpiresAt                 time.Time
	ConsumedAt                *time.Time
	CreatedAt                 time.Time
}
type DevicePlatform string

const (
	PlatformAndroid DevicePlatform = "android"
	PlatformIOS     DevicePlatform = "ios"
	PlatformWeb     DevicePlatform = "web"
	PlatformDesktop DevicePlatform = "desktop"
)

type DeviceInfo struct {
	Platform       DevicePlatform
	DeviceName     string
	PushToken      *string
	AppVersion     string
	InstallationID *string
}
type Device struct {
	ID, UserID string
	Platform   DevicePlatform
}
type Session struct {
	ID, UserID, DeviceID, RefreshTokenHash string
	AccessTokenJTI                         *string
	IPAddress, UserAgent                   string
	ExpiresAt                              time.Time
	RevokedAt                              *time.Time
	CreatedAt                              time.Time
}
type AuthResult struct {
	UserID                    string
	IsNewUser                 bool
	AccessToken, RefreshToken string
	AccessTokenExpiresAt      time.Time
}
