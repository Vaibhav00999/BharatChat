package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/bharatchat/backend/pkg/apperror"
	"io"
	"strings"
)

type AccountDeletionAuthorizer interface {
	AuthorizeAccountDeletion(ctx context.Context, userID, sessionID, refreshToken string) error
}

type UserService struct {
	repo                      domain.UserRepository
	accountDeletionAuthorizer AccountDeletionAuthorizer
}

func NewUserService(r domain.UserRepository) *UserService { return &UserService{repo: r} }

func (s *UserService) UseAccountDeletionAuthorizer(authorizer AccountDeletionAuthorizer) {
	s.accountDeletionAuthorizer = authorizer
}
func (s *UserService) FindOrCreateByPhone(ctx context.Context, phone, country string) (string, bool, error) {
	u, created, err := s.repo.FindOrCreateByPhone(ctx, phone, country)
	if err != nil {
		return "", false, apperror.NewInternal(err)
	}
	return u.ID, created, nil
}
func (s *UserService) GetProfile(ctx context.Context, id string) (*domain.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if u == nil {
		return nil, apperror.NewNotFound("user not found")
	}
	return u, nil
}
func (s *UserService) IsUsernameAvailable(ctx context.Context, name string) (bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !usernamePattern.MatchString(name) {
		return false, apperror.NewValidation("username must contain 3-30 letters or digits")
	}
	taken, err := s.repo.IsUsernameTaken(ctx, name)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return !taken, nil
}
func (s *UserService) UpdateProfile(ctx context.Context, id string, u domain.ProfileUpdate) (*domain.User, error) {
	if u.Username != nil {
		normalized := strings.ToLower(strings.TrimSpace(*u.Username))
		available, err := s.IsUsernameAvailable(ctx, normalized)
		if err != nil {
			return nil, err
		}
		if !available {
			current, err := s.GetProfile(ctx, id)
			if err != nil {
				return nil, err
			}
			if current.Username == nil || !strings.EqualFold(*current.Username, normalized) {
				return nil, apperror.NewConflict("username is already taken")
			}
		}
		u.Username = &normalized
	}
	if u.DisplayName != nil {
		v := strings.TrimSpace(*u.DisplayName)
		if v == "" {
			return nil, apperror.NewValidation("display name cannot be empty")
		}
		u.DisplayName = &v
	}
	out, err := s.repo.UpdateProfile(ctx, id, u)
	if errors.Is(err, domain.ErrUsernameTaken) {
		return nil, apperror.NewConflict("username is already taken")
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if out == nil {
		return nil, apperror.NewNotFound("user not found")
	}
	return out, nil
}

func validPrivacyLevel(level domain.PrivacyLevel) bool {
	return level == domain.PrivacyEveryone || level == domain.PrivacyContacts || level == domain.PrivacyNobody
}

func (s *UserService) UpdatePrivacy(ctx context.Context, id string, update domain.PrivacyUpdate) (*domain.User, error) {
	levels := []*domain.PrivacyLevel{update.LastSeen, update.Avatar, update.About, update.Phone, update.AllowGroupAdds}
	for _, level := range levels {
		if level != nil && !validPrivacyLevel(*level) {
			return nil, apperror.NewValidation("privacy values must be everyone, contacts, or nobody")
		}
	}
	user, err := s.repo.UpdatePrivacy(ctx, id, update)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if user == nil {
		return nil, apperror.NewNotFound("user not found")
	}
	return user, nil
}

func (s *UserService) CanBeAddedToGroup(ctx context.Context, requesterUserID, targetUserID string) (bool, error) {
	allowed, err := s.repo.CanBeAddedToGroup(ctx, requesterUserID, targetUserID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return allowed, nil
}

func (s *UserService) CanStartDirectChat(ctx context.Context, requesterUserID, targetUserID string) (bool, error) {
	allowed, err := s.repo.CanStartDirectChat(ctx, requesterUserID, targetUserID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return allowed, nil
}

func (s *UserService) ReadReceiptsEnabled(ctx context.Context, userID string) (bool, error) {
	enabled, err := s.repo.ReadReceiptsEnabled(ctx, userID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return enabled, nil
}

func (s *UserService) TypingIndicatorsEnabled(ctx context.Context, userID string) (bool, error) {
	enabled, err := s.repo.TypingIndicatorsEnabled(ctx, userID)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return enabled, nil
}

func (s *UserService) DeleteAccount(ctx context.Context, userID, sessionID, refreshToken, confirmation string) error {
	if confirmation != "DELETE" {
		return apperror.NewValidation("account deletion confirmation is invalid")
	}
	if s.accountDeletionAuthorizer == nil {
		return apperror.NewInternal(errors.New("account deletion authorizer is not configured"))
	}
	if err := s.accountDeletionAuthorizer.AuthorizeAccountDeletion(ctx, userID, sessionID, refreshToken); err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(refreshToken))
	deleted, err := s.repo.DeleteAccount(ctx, userID, sessionID, hex.EncodeToString(hash[:]))
	if errors.Is(err, domain.ErrDeletionSessionChanged) {
		return apperror.NewUnauthorized("current session confirmation is required")
	}
	if errors.Is(err, domain.ErrGroupOwnershipTransferRequired) {
		return apperror.NewConflict("transfer group ownership before deleting your account")
	}
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !deleted {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

func (s *UserService) ExportAccount(ctx context.Context, userID string, destination io.Writer) error {
	exporter, ok := s.repo.(domain.AccountExportRepository)
	if !ok {
		return apperror.NewInternal(errors.New("account export repository is not configured"))
	}
	found, err := exporter.WriteAccountExport(ctx, userID, destination)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !found {
		return apperror.NewNotFound("user not found")
	}
	return nil
}
