package service

import (
	"context"
	"crypto/ed25519"

	"github.com/bharatchat/backend/internal/features/privacy/domain"
	"github.com/bharatchat/backend/pkg/apperror"
)

const (
	minimumOneTimePreKeys = 10
	maximumOneTimePreKeys = 100
)

type PrivacyService struct {
	repo domain.KeyRepository
}

func NewPrivacyService(repo domain.KeyRepository) *PrivacyService {
	return &PrivacyService{repo: repo}
}

func (s *PrivacyService) UploadKeyBundle(ctx context.Context, sessionID, userID string, bundle domain.DeviceKeyBundle) (*domain.DeviceKeyBundle, error) {
	if len(bundle.IdentityAgreementPublicKey) != 32 || len(bundle.IdentitySigningPublicKey) != ed25519.PublicKeySize || len(bundle.SignedPreKeyPublicKey) != 32 || len(bundle.SignedPreKeySignature) != ed25519.SignatureSize {
		return nil, apperror.NewValidation("invalid public key bundle")
	}
	if bundle.RegistrationID < 1 || bundle.RegistrationID > 16380 || bundle.SignedPreKeyID < 0 || bundle.KeyVersion < 1 {
		return nil, apperror.NewValidation("invalid key identifiers")
	}
	if !ed25519.Verify(ed25519.PublicKey(bundle.IdentitySigningPublicKey), bundle.SignedPreKeyPublicKey, bundle.SignedPreKeySignature) {
		return nil, apperror.NewValidation("signed pre-key signature is invalid")
	}
	if len(bundle.OneTimePreKeys) < minimumOneTimePreKeys || len(bundle.OneTimePreKeys) > maximumOneTimePreKeys {
		return nil, apperror.NewValidation("provide between 10 and 100 one-time pre-keys")
	}
	seen := make(map[int]struct{}, len(bundle.OneTimePreKeys))
	for _, key := range bundle.OneTimePreKeys {
		if key.KeyID < 0 || len(key.PublicKey) != 32 {
			return nil, apperror.NewValidation("invalid one-time pre-key")
		}
		if _, exists := seen[key.KeyID]; exists {
			return nil, apperror.NewValidation("one-time pre-key IDs must be unique")
		}
		seen[key.KeyID] = struct{}{}
	}

	stored, err := s.repo.UpsertBundle(ctx, sessionID, userID, bundle)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if stored == nil {
		return nil, apperror.NewUnauthorized("session is no longer active")
	}
	return stored, nil
}

func (s *PrivacyService) ClaimBundles(ctx context.Context, requesterSessionID, requesterUserID, targetUserID string) ([]domain.DeviceKeyBundle, error) {
	bundles, err := s.repo.ClaimBundlesForUser(ctx, requesterSessionID, requesterUserID, targetUserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if len(bundles) == 0 {
		return nil, apperror.NewForbidden("key bundles are available only to active conversation participants")
	}
	return bundles, nil
}

func (s *PrivacyService) ResolveDeviceID(ctx context.Context, sessionID, userID string) (string, error) {
	deviceID, err := s.repo.ResolveDeviceID(ctx, sessionID, userID)
	if err != nil {
		return "", apperror.NewInternal(err)
	}
	if deviceID == "" {
		return "", apperror.NewUnauthorized("session is no longer active")
	}
	return deviceID, nil
}

func (s *PrivacyService) ListLinkedDevices(ctx context.Context, userID string) ([]domain.LinkedDevice, error) {
	devices, err := s.repo.ListLinkedDevices(ctx, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return devices, nil
}

func (s *PrivacyService) RevokeLinkedDevice(ctx context.Context, currentSessionID, userID, deviceID string) error {
	currentDeviceID, err := s.ResolveDeviceID(ctx, currentSessionID, userID)
	if err != nil {
		return err
	}
	if currentDeviceID == deviceID {
		return apperror.NewValidation("log out to unlink the current device")
	}
	revoked, err := s.repo.RevokeLinkedDevice(ctx, userID, deviceID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !revoked {
		return apperror.NewNotFound("linked device not found")
	}
	return nil
}
