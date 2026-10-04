package domain

import "time"

type OneTimePreKey struct {
	KeyID     int
	PublicKey []byte
}

// DeviceKeyBundle contains only public key material. The corresponding private
// keys are generated and retained in platform secure storage on the client.
type DeviceKeyBundle struct {
	DeviceID                   string
	UserID                     string
	RegistrationID             int
	IdentityAgreementPublicKey []byte
	IdentitySigningPublicKey   []byte
	SignedPreKeyID             int
	SignedPreKeyPublicKey      []byte
	SignedPreKeySignature      []byte
	KeyVersion                 int
	OneTimePreKey              *OneTimePreKey
	OneTimePreKeys             []OneTimePreKey
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type LinkedDevice struct {
	DeviceID       string
	Platform       string
	DeviceName     *string
	LastActiveAt   time.Time
	HasKeyBundle   bool
	ActiveSessions int
}
