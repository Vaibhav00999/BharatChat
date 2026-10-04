package service

import (
	"context"
	"github.com/bharatchat/backend/internal/features/chat/domain"
	"github.com/bharatchat/backend/pkg/apperror"
)

type DirectChatPrivacyAuthorizer interface {
	CanStartDirectChat(ctx context.Context, requesterUserID, targetUserID string) (bool, error)
}

type ChatService struct {
	repo    domain.ChatRepository
	privacy DirectChatPrivacyAuthorizer
}

func NewChatService(r domain.ChatRepository, privacy ...DirectChatPrivacyAuthorizer) *ChatService {
	service := &ChatService{repo: r}
	if len(privacy) > 0 {
		service.privacy = privacy[0]
	}
	return service
}
func (s *ChatService) StartDirectChat(ctx context.Context, user, peer string) (*domain.Chat, error) {
	if user == peer {
		return nil, apperror.NewValidation("cannot start a chat with yourself")
	}
	if s.privacy != nil {
		allowed, err := s.privacy.CanStartDirectChat(ctx, user, peer)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, apperror.NewForbidden("direct chat is not available")
		}
	}
	c, err := s.repo.FindOrCreateDirectChat(ctx, user, peer)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return c, nil
}
func (s *ChatService) ListChats(ctx context.Context, user string) ([]domain.ChatSummary, error) {
	v, err := s.repo.ListChatSummariesForUser(ctx, user)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return v, nil
}
func (s *ChatService) FindByID(ctx context.Context, id string) (*domain.Chat, error) {
	c, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return c, nil
}
func (s *ChatService) AuthorizeParticipant(ctx context.Context, chat, user string) error {
	c, err := s.repo.FindByID(ctx, chat)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if c == nil {
		return apperror.NewNotFound("chat not found")
	}
	ok, err := s.repo.IsParticipant(ctx, chat, user)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewForbidden("you are not a participant of this chat")
	}
	return nil
}
func (s *ChatService) ListParticipants(ctx context.Context, id string) ([]domain.Participant, error) {
	v, err := s.repo.ListParticipants(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return v, nil
}

// AuthorizeInteraction applies current blocking policy to sending and typing.
// History remains available to the participants after they block each other.
func (s *ChatService) AuthorizeInteraction(ctx context.Context, chat, user string) error {
	if err := s.AuthorizeParticipant(ctx, chat, user); err != nil {
		return err
	}
	c, err := s.FindByID(ctx, chat)
	if err != nil {
		return err
	}
	if c == nil {
		return apperror.NewNotFound("chat not found")
	}
	if c.Type != domain.ChatTypeDirect || s.privacy == nil {
		return nil
	}
	participants, err := s.ListParticipants(ctx, chat)
	if err != nil {
		return err
	}
	for _, p := range participants {
		if p.UserID == user {
			continue
		}
		allowed, err := s.privacy.CanStartDirectChat(ctx, user, p.UserID)
		if err != nil {
			return err
		}
		if !allowed {
			return apperror.NewForbidden("direct chat is not available")
		}
	}
	return nil
}
func (s *ChatService) SetMuted(ctx context.Context, c, u string, v bool) error {
	if err := s.AuthorizeParticipant(ctx, c, u); err != nil {
		return err
	}
	if err := s.repo.SetMuted(ctx, c, u, v); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
func (s *ChatService) SetPinned(ctx context.Context, c, u string, v bool) error {
	if err := s.AuthorizeParticipant(ctx, c, u); err != nil {
		return err
	}
	if err := s.repo.SetPinned(ctx, c, u, v); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
func (s *ChatService) SetArchived(ctx context.Context, c, u string, v bool) error {
	if err := s.AuthorizeParticipant(ctx, c, u); err != nil {
		return err
	}
	if err := s.repo.SetArchived(ctx, c, u, v); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
func (s *ChatService) CreateGroupChat(ctx context.Context, creator string, members []string) (*domain.Chat, error) {
	unique := map[string]struct{}{creator: {}}
	for _, id := range members {
		unique[id] = struct{}{}
	}
	if len(unique) < 2 {
		return nil, apperror.NewValidation("a group must have at least one other member")
	}
	if len(unique) > 256 {
		return nil, apperror.NewValidation("group cannot exceed 256 members")
	}
	ids := make([]string, 0, len(unique)-1)
	for id := range unique {
		if id != creator {
			ids = append(ids, id)
		}
	}
	c, err := s.repo.CreateGroupChat(ctx, creator, ids)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return c, nil
}

func (s *ChatService) DeleteGroupChat(ctx context.Context, chatID string) error {
	if err := s.repo.DeleteGroupChat(ctx, chatID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
func (s *ChatService) GetParticipantRole(ctx context.Context, c, u string) (domain.MemberRole, bool, error) {
	r, ok, err := s.repo.GetParticipantRole(ctx, c, u)
	if err != nil {
		return "", false, apperror.NewInternal(err)
	}
	return r, ok, nil
}
func (s *ChatService) UpdateParticipantRole(ctx context.Context, c, u string, r domain.MemberRole) error {
	if err := s.repo.UpdateParticipantRole(ctx, c, u, r); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
func (s *ChatService) AddParticipant(ctx context.Context, c, u string, r domain.MemberRole) error {
	if repo, ok := s.repo.(interface {
		AddParticipantIfBelowLimit(context.Context, string, string, domain.MemberRole, int) (bool, error)
	}); ok {
		added, err := repo.AddParticipantIfBelowLimit(ctx, c, u, r, 256)
		if err != nil {
			return apperror.NewInternal(err)
		}
		if !added {
			return apperror.NewValidation("group has reached its maximum member limit")
		}
		return nil
	}

	count, err := s.repo.CountParticipants(ctx, c)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if count >= 256 {
		return apperror.NewValidation("group has reached its maximum member limit")
	}
	if err := s.repo.AddParticipant(ctx, c, u, r); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *ChatService) TransferOwnership(ctx context.Context, chatID, currentOwnerUserID, newOwnerUserID string) error {
	repo, ok := s.repo.(interface {
		TransferOwnership(context.Context, string, string, string) error
	})
	if !ok {
		if err := s.repo.UpdateParticipantRole(ctx, chatID, newOwnerUserID, domain.RoleOwner); err != nil {
			return apperror.NewInternal(err)
		}
		if err := s.repo.UpdateParticipantRole(ctx, chatID, currentOwnerUserID, domain.RoleAdmin); err != nil {
			return apperror.NewInternal(err)
		}
		return nil
	}
	if err := repo.TransferOwnership(ctx, chatID, currentOwnerUserID, newOwnerUserID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
func (s *ChatService) RemoveParticipant(ctx context.Context, c, u string) error {
	if err := s.repo.RemoveParticipant(ctx, c, u); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
