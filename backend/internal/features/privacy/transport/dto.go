package transport

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/bharatchat/backend/internal/features/privacy/domain"
)

type OneTimePreKeyRequest struct {
	KeyID     int    `json:"keyId" validate:"gte=0"`
	PublicKey string `json:"publicKey" validate:"required"`
}

type UploadKeyBundleRequest struct {
	RegistrationID             int                    `json:"registrationId" validate:"required,min=1,max=16380"`
	IdentityAgreementPublicKey string                 `json:"identityAgreementPublicKey" validate:"required"`
	IdentitySigningPublicKey   string                 `json:"identitySigningPublicKey" validate:"required"`
	SignedPreKeyID             int                    `json:"signedPreKeyId" validate:"gte=0"`
	SignedPreKeyPublicKey      string                 `json:"signedPreKeyPublicKey" validate:"required"`
	SignedPreKeySignature      string                 `json:"signedPreKeySignature" validate:"required"`
	KeyVersion                 int                    `json:"keyVersion" validate:"required,min=1"`
	OneTimePreKeys             []OneTimePreKeyRequest `json:"oneTimePreKeys" validate:"required,min=10,max=100,dive"`
}

func decodeKey(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	decoded, stdErr := base64.StdEncoding.DecodeString(value)
	if stdErr != nil {
		return nil, fmt.Errorf("invalid base64url key: %w", err)
	}
	return decoded, nil
}

func (r UploadKeyBundleRequest) ToDomain() (domain.DeviceKeyBundle, error) {
	agreement, err := decodeKey(r.IdentityAgreementPublicKey)
	if err != nil {
		return domain.DeviceKeyBundle{}, err
	}
	signing, err := decodeKey(r.IdentitySigningPublicKey)
	if err != nil {
		return domain.DeviceKeyBundle{}, err
	}
	signedPreKey, err := decodeKey(r.SignedPreKeyPublicKey)
	if err != nil {
		return domain.DeviceKeyBundle{}, err
	}
	signature, err := decodeKey(r.SignedPreKeySignature)
	if err != nil {
		return domain.DeviceKeyBundle{}, err
	}

	preKeys := make([]domain.OneTimePreKey, 0, len(r.OneTimePreKeys))
	for _, input := range r.OneTimePreKeys {
		publicKey, err := decodeKey(input.PublicKey)
		if err != nil {
			return domain.DeviceKeyBundle{}, err
		}
		preKeys = append(preKeys, domain.OneTimePreKey{KeyID: input.KeyID, PublicKey: publicKey})
	}
	return domain.DeviceKeyBundle{
		RegistrationID: r.RegistrationID, IdentityAgreementPublicKey: agreement,
		IdentitySigningPublicKey: signing, SignedPreKeyID: r.SignedPreKeyID,
		SignedPreKeyPublicKey: signedPreKey, SignedPreKeySignature: signature,
		KeyVersion: r.KeyVersion, OneTimePreKeys: preKeys,
	}, nil
}

type OneTimePreKeyResponse struct {
	KeyID     int    `json:"keyId"`
	PublicKey string `json:"publicKey"`
}

type KeyBundleResponse struct {
	DeviceID                   string                 `json:"deviceId"`
	UserID                     string                 `json:"userId"`
	RegistrationID             int                    `json:"registrationId"`
	IdentityAgreementPublicKey string                 `json:"identityAgreementPublicKey"`
	IdentitySigningPublicKey   string                 `json:"identitySigningPublicKey"`
	SignedPreKeyID             int                    `json:"signedPreKeyId"`
	SignedPreKeyPublicKey      string                 `json:"signedPreKeyPublicKey"`
	SignedPreKeySignature      string                 `json:"signedPreKeySignature"`
	KeyVersion                 int                    `json:"keyVersion"`
	OneTimePreKey              *OneTimePreKeyResponse `json:"oneTimePreKey,omitempty"`
	UpdatedAt                  time.Time              `json:"updatedAt"`
}

func toKeyBundleResponse(bundle domain.DeviceKeyBundle) KeyBundleResponse {
	encode := base64.RawURLEncoding.EncodeToString
	response := KeyBundleResponse{
		DeviceID: bundle.DeviceID, UserID: bundle.UserID, RegistrationID: bundle.RegistrationID,
		IdentityAgreementPublicKey: encode(bundle.IdentityAgreementPublicKey),
		IdentitySigningPublicKey:   encode(bundle.IdentitySigningPublicKey),
		SignedPreKeyID:             bundle.SignedPreKeyID, SignedPreKeyPublicKey: encode(bundle.SignedPreKeyPublicKey),
		SignedPreKeySignature: encode(bundle.SignedPreKeySignature), KeyVersion: bundle.KeyVersion,
		UpdatedAt: bundle.UpdatedAt,
	}
	if bundle.OneTimePreKey != nil {
		response.OneTimePreKey = &OneTimePreKeyResponse{KeyID: bundle.OneTimePreKey.KeyID, PublicKey: encode(bundle.OneTimePreKey.PublicKey)}
	}
	return response
}

type LinkedDeviceResponse struct {
	DeviceID       string    `json:"deviceId"`
	Platform       string    `json:"platform"`
	DeviceName     *string   `json:"deviceName,omitempty"`
	LastActiveAt   time.Time `json:"lastActiveAt"`
	HasKeyBundle   bool      `json:"hasKeyBundle"`
	ActiveSessions int       `json:"activeSessions"`
	IsCurrent      bool      `json:"isCurrent"`
}
