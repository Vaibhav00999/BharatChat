package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/stretchr/testify/require"
)

type deletionRepoStub struct {
	domain.UserRepository
	delete func(context.Context, string, string, string) (bool, error)
}

func (s deletionRepoStub) DeleteAccount(ctx context.Context, user, session, hash string) (bool, error) {
	return s.delete(ctx, user, session, hash)
}

type deletionAuthorizerStub struct{ err error }

func (s deletionAuthorizerStub) AuthorizeAccountDeletion(context.Context, string, string, string) error {
	return s.err
}

func TestDeletionPassesSessionAndHashedCredentialToTransaction(t *testing.T) {
	repo := deletionRepoStub{delete: func(ctx context.Context, user, session, hash string) (bool, error) {
		require.Equal(t, "user-id", user)
		require.Equal(t, "session-id", session)
		digest := sha256.Sum256([]byte("current-secret"))
		require.Equal(t, hex.EncodeToString(digest[:]), hash)
		require.NotEqual(t, "current-secret", hash)
		return true, nil
	}}
	svc := NewUserService(repo)
	svc.UseAccountDeletionAuthorizer(deletionAuthorizerStub{})
	require.NoError(t, svc.DeleteAccount(context.Background(), "user-id", "session-id", "current-secret", "DELETE"))
}

func TestDeletionMapsTransactionalGuards(t *testing.T) {
	for _, test := range []struct {
		name   string
		cause  error
		status int
	}{
		{"rotated-session", domain.ErrDeletionSessionChanged, http.StatusUnauthorized},
		{"group-owner", domain.ErrGroupOwnershipTransferRequired, http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := NewUserService(deletionRepoStub{delete: func(context.Context, string, string, string) (bool, error) { return false, test.cause }})
			svc.UseAccountDeletionAuthorizer(deletionAuthorizerStub{})
			err := svc.DeleteAccount(context.Background(), "user", "session", "secret", "DELETE")
			appErr, ok := apperror.As(err)
			require.True(t, ok)
			require.Equal(t, test.status, appErr.HTTPStatus)
		})
	}
}

func TestDeletionFailsClosedWithoutAuthorizer(t *testing.T) {
	svc := NewUserService(deletionRepoStub{delete: func(context.Context, string, string, string) (bool, error) {
		t.Fatal("repository called before step-up confirmation")
		return false, nil
	}})
	require.Error(t, svc.DeleteAccount(context.Background(), "user", "session", "secret", "DELETE"))
	svc.UseAccountDeletionAuthorizer(deletionAuthorizerStub{err: apperror.NewUnauthorized("invalid session")})
	require.Error(t, svc.DeleteAccount(context.Background(), "user", "session", "secret", "DELETE"))
}
