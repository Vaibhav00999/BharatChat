package service_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/bharatchat/backend/internal/features/privacy/domain"
	"github.com/bharatchat/backend/internal/features/privacy/service"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/stretchr/testify/require"
)

type fakeKeyRepository struct {
	stored          *domain.DeviceKeyBundle
	resolvedDevice  string
	revokedDeviceID string
}

func (f *fakeKeyRepository) UpsertBundle(_ context.Context, _, userID string, bundle domain.DeviceKeyBundle) (*domain.DeviceKeyBundle, error) {
	bundle.UserID = userID
	bundle.DeviceID = f.resolvedDevice
	f.stored = &bundle
	return &bundle, nil
}
func (f *fakeKeyRepository) ClaimBundlesForUser(context.Context, string, string, string) ([]domain.DeviceKeyBundle, error) {
	if f.stored == nil {
		return nil, nil
	}
	return []domain.DeviceKeyBundle{*f.stored}, nil
}
func (f *fakeKeyRepository) ResolveDeviceID(context.Context, string, string) (string, error) {
	return f.resolvedDevice, nil
}
func (f *fakeKeyRepository) ListLinkedDevices(context.Context, string) ([]domain.LinkedDevice, error) {
	return nil, nil
}
func (f *fakeKeyRepository) RevokeLinkedDevice(_ context.Context, _ string, deviceID string) (bool, error) {
	f.revokedDeviceID = deviceID
	return true, nil
}

func validBundle(t *testing.T) domain.DeviceKeyBundle {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signedPreKey := make([]byte, 32)
	_, err = rand.Read(signedPreKey)
	require.NoError(t, err)
	bundle := domain.DeviceKeyBundle{
		RegistrationID: 1, IdentityAgreementPublicKey: make([]byte, 32),
		IdentitySigningPublicKey: publicKey, SignedPreKeyID: 1,
		SignedPreKeyPublicKey: signedPreKey,
		SignedPreKeySignature: ed25519.Sign(privateKey, signedPreKey), KeyVersion: 1,
	}
	for keyID := 0; keyID < 10; keyID++ {
		bundle.OneTimePreKeys = append(bundle.OneTimePreKeys, domain.OneTimePreKey{KeyID: keyID, PublicKey: make([]byte, 32)})
	}
	return bundle
}

func TestUploadKeyBundleAcceptsValidSignedBundle(t *testing.T) {
	repo := &fakeKeyRepository{resolvedDevice: "device-1"}
	svc := service.NewPrivacyService(repo)

	stored, err := svc.UploadKeyBundle(context.Background(), "session-1", "user-1", validBundle(t))

	require.NoError(t, err)
	require.Equal(t, "device-1", stored.DeviceID)
	require.Len(t, stored.OneTimePreKeys, 10)
}

func TestUploadKeyBundleRejectsForgedSignedPreKey(t *testing.T) {
	repo := &fakeKeyRepository{resolvedDevice: "device-1"}
	svc := service.NewPrivacyService(repo)
	bundle := validBundle(t)
	bundle.SignedPreKeyPublicKey[0] ^= 0xff

	_, err := svc.UploadKeyBundle(context.Background(), "session-1", "user-1", bundle)

	require.Error(t, err)
	appErr, ok := apperror.As(err)
	require.True(t, ok)
	require.Equal(t, "VALIDATION_ERROR", appErr.Code)
	require.Nil(t, repo.stored)
}

func TestRevokeLinkedDeviceRejectsCurrentDevice(t *testing.T) {
	repo := &fakeKeyRepository{resolvedDevice: "device-1"}
	svc := service.NewPrivacyService(repo)

	err := svc.RevokeLinkedDevice(context.Background(), "session-1", "user-1", "device-1")

	require.Error(t, err)
	require.Empty(t, repo.revokedDeviceID)
}
