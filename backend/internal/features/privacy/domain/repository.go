package domain

import "context"

type KeyRepository interface {
	UpsertBundle(ctx context.Context, sessionID, userID string, bundle DeviceKeyBundle) (*DeviceKeyBundle, error)
	ClaimBundlesForUser(ctx context.Context, requesterSessionID, requesterUserID, targetUserID string) ([]DeviceKeyBundle, error)
	ResolveDeviceID(ctx context.Context, sessionID, userID string) (string, error)
	ListLinkedDevices(ctx context.Context, userID string) ([]LinkedDevice, error)
	RevokeLinkedDevice(ctx context.Context, ownerUserID, deviceID string) (bool, error)
}
