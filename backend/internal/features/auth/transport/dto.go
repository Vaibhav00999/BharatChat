package transport

type RequestOTPRequest struct {
	PhoneNumber string `json:"phoneNumber" validate:"required,e164"`
	CountryCode string `json:"countryCode" validate:"required,min=1,max=5"`
}
type VerifyOTPRequest struct {
	PhoneNumber string        `json:"phoneNumber" validate:"required,e164"`
	CountryCode string        `json:"countryCode" validate:"required,min=1,max=5"`
	Code        string        `json:"code" validate:"required,len=6,numeric"`
	Device      DeviceInfoDTO `json:"device" validate:"required"`
}
type DeviceInfoDTO struct {
	Platform       string  `json:"platform" validate:"required,oneof=android ios web desktop"`
	DeviceName     string  `json:"deviceName" validate:"required,max=120"`
	PushToken      *string `json:"pushToken,omitempty"`
	AppVersion     string  `json:"appVersion" validate:"required,max=20"`
	InstallationID *string `json:"installationId,omitempty" validate:"omitempty,uuid4"`
}
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}
type AuthResponse struct {
	UserID               string `json:"userId"`
	IsNewUser            bool   `json:"isNewUser"`
	AccessToken          string `json:"accessToken"`
	RefreshToken         string `json:"refreshToken"`
	AccessTokenExpiresAt string `json:"accessTokenExpiresAt"`
}
