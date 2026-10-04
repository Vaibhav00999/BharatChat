package domain

import (
	"context"
	"errors"
)

var ErrDeletionSessionChanged = errors.New("account deletion session changed")
var ErrGroupOwnershipTransferRequired = errors.New("group ownership transfer required")

type UserRepository interface {
	FindByPhone(context.Context, string) (*User, error)
	FindByID(context.Context, string) (*User, error)
	FindOrCreateByPhone(context.Context, string, string) (*User, bool, error)
	IsUsernameTaken(context.Context, string) (bool, error)
	UpdateProfile(context.Context, string, ProfileUpdate) (*User, error)
	UpdatePrivacy(context.Context, string, PrivacyUpdate) (*User, error)
	CanBeAddedToGroup(context.Context, string, string) (bool, error)
	CanStartDirectChat(context.Context, string, string) (bool, error)
	ReadReceiptsEnabled(context.Context, string) (bool, error)
	TypingIndicatorsEnabled(context.Context, string) (bool, error)
	DeleteAccount(ctx context.Context, userID, sessionID, currentRefreshHash string) (bool, error)
}
