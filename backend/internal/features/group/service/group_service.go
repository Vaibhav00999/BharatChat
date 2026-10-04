// internal/features/group/service/group_service.go
package service

import (
	"context"
	"strings"

	chatDomain "github.com/bharatchat/backend/internal/features/chat/domain"
	"github.com/bharatchat/backend/internal/features/group/domain"
	"github.com/bharatchat/backend/pkg/apperror"
)

// ChatGroupCreator is the slice of chat/service.ChatService the group feature
// needs to actually create the underlying chat + participant rows — group/service
// depends on this narrow interface, not chat/service's full surface, keeping the
// group -> chat dependency arrow intentional and minimal.
type ChatGroupCreator interface {
	CreateGroupChat(ctx context.Context, creatorUserID string, memberUserIDs []string) (*chatDomain.Chat, error)
	DeleteGroupChat(ctx context.Context, chatID string) error
	GetParticipantRole(ctx context.Context, chatID, userID string) (chatDomain.MemberRole, bool, error)
	UpdateParticipantRole(ctx context.Context, chatID, userID string, role chatDomain.MemberRole) error
	AddParticipant(ctx context.Context, chatID, userID string, role chatDomain.MemberRole) error
	RemoveParticipant(ctx context.Context, chatID, userID string) error
	TransferOwnership(ctx context.Context, chatID, currentOwnerUserID, newOwnerUserID string) error
	AuthorizeParticipant(ctx context.Context, chatID, userID string) error
	FindByID(ctx context.Context, chatID string) (*chatDomain.Chat, error)
	ListParticipants(ctx context.Context, chatID string) ([]chatDomain.Participant, error)
}

type MembershipPrivacyAuthorizer interface {
	CanBeAddedToGroup(ctx context.Context, requesterUserID, targetUserID string) (bool, error)
	CanStartDirectChat(ctx context.Context, requesterUserID, targetUserID string) (bool, error)
}

type GroupService struct {
	repo    domain.GroupRepository
	chat    ChatGroupCreator
	privacy MembershipPrivacyAuthorizer
}

func NewGroupService(repo domain.GroupRepository, chat ChatGroupCreator, privacy ...MembershipPrivacyAuthorizer) *GroupService {
	service := &GroupService{repo: repo, chat: chat}
	if len(privacy) > 0 {
		service.privacy = privacy[0]
	}
	return service
}

type CreateGroupInput struct {
	CreatorUserID string
	Name          string
	Description   string
	MemberUserIDs []string
}

func (s *GroupService) CreateGroup(ctx context.Context, input CreateGroupInput) (*domain.GroupInfo, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewValidation("group name is required")
	}
	if len(name) > 120 {
		return nil, apperror.NewValidation("group name is too long")
	}
	if s.privacy != nil {
		for _, memberUserID := range input.MemberUserIDs {
			if memberUserID == input.CreatorUserID {
				continue
			}
			allowed, err := s.privacy.CanBeAddedToGroup(ctx, input.CreatorUserID, memberUserID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				return nil, apperror.NewForbidden("one or more users do not allow this group invitation")
			}
		}
	}

	chat, err := s.chat.CreateGroupChat(ctx, input.CreatorUserID, input.MemberUserIDs)
	if err != nil {
		return nil, err // already an *apperror.AppError from chat/service
	}

	var description *string
	if trimmed := strings.TrimSpace(input.Description); trimmed != "" {
		description = &trimmed
	}

	group, err := s.repo.Create(ctx, domain.GroupInfo{
		ChatID:      chat.ID,
		Name:        name,
		Description: description,
	})
	if err != nil {
		_ = s.chat.DeleteGroupChat(ctx, chat.ID)
		return nil, apperror.NewInternal(err)
	}

	return group, nil
}

func (s *GroupService) GetGroupInfo(ctx context.Context, chatID, requestingUserID string) (*domain.GroupInfo, error) {
	if err := s.chat.AuthorizeParticipant(ctx, chatID, requestingUserID); err != nil {
		return nil, err
	}

	group, err := s.repo.FindByChatID(ctx, chatID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if group == nil {
		return nil, apperror.NewNotFound("group not found")
	}
	return s.visibleGroupInfo(ctx, group, requestingUserID)
}

func (s *GroupService) visibleGroupInfo(ctx context.Context, group *domain.GroupInfo, requestingUserID string) (*domain.GroupInfo, error) {
	role, _, err := s.chat.GetParticipantRole(ctx, group.ChatID, requestingUserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if role == chatDomain.RoleMember {
		visible := *group
		visible.InviteCode = nil
		return &visible, nil
	}
	return group, nil
}

func (s *GroupService) ListMembers(ctx context.Context, chatID, requestingUserID string) ([]domain.MemberView, error) {
	if err := s.chat.AuthorizeParticipant(ctx, chatID, requestingUserID); err != nil {
		return nil, err
	}

	members, err := s.repo.ListMembers(ctx, chatID, requestingUserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return members, nil
}

// requireRole is the single permission-check primitive every mutating group
// operation below funnels through — centralizing "what role can do what" here
// means the table in this module's architecture section maps to exactly one
// function, not scattered ad-hoc checks per handler.
func (s *GroupService) requireRole(ctx context.Context, chatID, userID string, allowed ...chatDomain.MemberRole) error {
	role, isMember, err := s.chat.GetParticipantRole(ctx, chatID, userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !isMember {
		return apperror.NewForbidden("you are not a member of this group")
	}
	for _, r := range allowed {
		if role == r {
			return nil
		}
	}
	return apperror.NewForbidden("you do not have permission to perform this action")
}

type UpdateGroupInfoInput struct {
	ChatID           string
	RequestingUserID string
	Name             *string
	Description      *string
	IconURL          *string
}

func (s *GroupService) UpdateGroupInfo(ctx context.Context, input UpdateGroupInfoInput) (*domain.GroupInfo, error) {
	group, err := s.repo.FindByChatID(ctx, input.ChatID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if group == nil {
		return nil, apperror.NewNotFound("group not found")
	}

	if group.OnlyAdminsCanEditInfo {
		if err := s.requireRole(ctx, input.ChatID, input.RequestingUserID, chatDomain.RoleOwner, chatDomain.RoleAdmin); err != nil {
			return nil, err
		}
	} else {
		if err := s.chat.AuthorizeParticipant(ctx, input.ChatID, input.RequestingUserID); err != nil {
			return nil, err
		}
	}

	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			return nil, apperror.NewValidation("group name cannot be empty")
		}
		input.Name = &trimmed
	}

	updated, err := s.repo.UpdateInfo(ctx, input.ChatID, input.Name, input.Description, input.IconURL)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if updated == nil {
		return nil, apperror.NewNotFound("group not found")
	}
	return s.visibleGroupInfo(ctx, updated, input.RequestingUserID)
}

func (s *GroupService) SetOnlyAdminsCanPost(ctx context.Context, chatID, requestingUserID string, value bool) error {
	if err := s.requireRole(ctx, chatID, requestingUserID, chatDomain.RoleOwner, chatDomain.RoleAdmin); err != nil {
		return err
	}
	if err := s.repo.SetOnlyAdminsCanPost(ctx, chatID, value); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *GroupService) SetOnlyAdminsCanEditInfo(ctx context.Context, chatID, requestingUserID string, value bool) error {
	if err := s.requireRole(ctx, chatID, requestingUserID, chatDomain.RoleOwner, chatDomain.RoleAdmin); err != nil {
		return err
	}
	if err := s.repo.SetOnlyAdminsCanEditInfo(ctx, chatID, value); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *GroupService) RegenerateInviteCode(ctx context.Context, chatID, requestingUserID string) (string, error) {
	if err := s.requireRole(ctx, chatID, requestingUserID, chatDomain.RoleOwner, chatDomain.RoleAdmin); err != nil {
		return "", err
	}
	code, err := s.repo.RegenerateInviteCode(ctx, chatID)
	if err != nil {
		return "", apperror.NewInternal(err)
	}
	return code, nil
}

func (s *GroupService) SetInviteCodeEnabled(ctx context.Context, chatID, requestingUserID string, enabled bool) error {
	if err := s.requireRole(ctx, chatID, requestingUserID, chatDomain.RoleOwner, chatDomain.RoleAdmin); err != nil {
		return err
	}
	if err := s.repo.SetInviteCodeEnabled(ctx, chatID, enabled); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// JoinViaInviteCode is the entry point for someone joining a group via a shared
// link — it looks up the group by code, then adds the requesting user as a plain
// member (never admin/owner, regardless of who shares the link).
func (s *GroupService) JoinViaInviteCode(ctx context.Context, inviteCode, requestingUserID string) (*domain.GroupInfo, error) {
	group, err := s.repo.FindByInviteCode(ctx, inviteCode)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if group == nil {
		return nil, apperror.NewNotFound("invalid or expired invite code")
	}

	if err := s.chat.AddParticipant(ctx, group.ChatID, requestingUserID, chatDomain.RoleMember); err != nil {
		return nil, err
	}

	return group, nil
}

func (s *GroupService) AddMember(ctx context.Context, chatID, requestingUserID, newMemberUserID string) error {
	if err := s.requireRole(ctx, chatID, requestingUserID, chatDomain.RoleOwner, chatDomain.RoleAdmin); err != nil {
		return err
	}
	if s.privacy != nil {
		allowed, err := s.privacy.CanBeAddedToGroup(ctx, requestingUserID, newMemberUserID)
		if err != nil {
			return err
		}
		if !allowed {
			return apperror.NewForbidden("this user does not allow group invitations from you")
		}
	}
	if err := s.chat.AddParticipant(ctx, chatID, newMemberUserID, chatDomain.RoleMember); err != nil {
		return err
	}
	return nil
}

// RemoveMember enforces: owner can remove anyone; admin can remove members and
// other admins but never the owner; a member cannot remove anyone.
func (s *GroupService) RemoveMember(ctx context.Context, chatID, requestingUserID, targetUserID string) error {
	requestingRole, isMember, err := s.chat.GetParticipantRole(ctx, chatID, requestingUserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !isMember {
		return apperror.NewForbidden("you are not a member of this group")
	}
	if requestingRole != chatDomain.RoleOwner && requestingRole != chatDomain.RoleAdmin {
		return apperror.NewForbidden("only owners and admins can remove members")
	}

	targetRole, targetIsMember, err := s.chat.GetParticipantRole(ctx, chatID, targetUserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !targetIsMember {
		return apperror.NewNotFound("that user is not a member of this group")
	}
	if targetRole == chatDomain.RoleOwner {
		return apperror.NewForbidden("the group owner cannot be removed")
	}
	if err := s.chat.RemoveParticipant(ctx, chatID, targetUserID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// LeaveGroup lets any member remove themselves, EXCEPT the owner, who must
// transfer ownership first (TransferOwnership below) — this prevents a group
// from ending up with zero owners, which the schema has no mechanism to recover from.
func (s *GroupService) LeaveGroup(ctx context.Context, chatID, userID string) error {
	role, isMember, err := s.chat.GetParticipantRole(ctx, chatID, userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !isMember {
		return apperror.NewForbidden("you are not a member of this group")
	}
	if role == chatDomain.RoleOwner {
		return apperror.NewValidation("transfer ownership to another member before leaving the group")
	}

	if err := s.chat.RemoveParticipant(ctx, chatID, userID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// TransferOwnership: only the current owner may call this; it promotes the target
// to owner and demotes the caller to admin (so they remain a group member with
// elevated privileges, rather than being silently downgraded to a plain member).
func (s *GroupService) TransferOwnership(ctx context.Context, chatID, currentOwnerUserID, newOwnerUserID string) error {
	if err := s.requireRole(ctx, chatID, currentOwnerUserID, chatDomain.RoleOwner); err != nil {
		return err
	}

	_, targetIsMember, err := s.chat.GetParticipantRole(ctx, chatID, newOwnerUserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !targetIsMember {
		return apperror.NewValidation("the new owner must already be a member of the group")
	}

	if err := s.chat.TransferOwnership(ctx, chatID, currentOwnerUserID, newOwnerUserID); err != nil {
		return err
	}
	return nil
}

// SetAdmin promotes/demotes between admin and member — only the owner may call this
// (admins cannot create or remove other admins, matching the permission table).
func (s *GroupService) SetAdmin(ctx context.Context, chatID, requestingUserID, targetUserID string, isAdmin bool) error {
	if err := s.requireRole(ctx, chatID, requestingUserID, chatDomain.RoleOwner); err != nil {
		return err
	}

	targetRole, targetIsMember, err := s.chat.GetParticipantRole(ctx, chatID, targetUserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !targetIsMember {
		return apperror.NewNotFound("that user is not a member of this group")
	}
	if targetRole == chatDomain.RoleOwner {
		return apperror.NewValidation("cannot change the owner's role")
	}

	newRole := chatDomain.RoleMember
	if isAdmin {
		newRole = chatDomain.RoleAdmin
	}

	if err := s.chat.UpdateParticipantRole(ctx, chatID, targetUserID, newRole); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// CanPost satisfies message/service.GroupPostingAuthorizer. It resolves the
// chat's type first — if it's not a group, posting is always allowed (this is
// how direct-chat sends stay completely unaffected by this module without
// message/service needing any chat-type awareness of its own).
func (s *GroupService) CanPost(ctx context.Context, chatID, userID string) (bool, error) {
	chat, err := s.chat.FindByID(ctx, chatID)
	if err != nil {
		return false, err
	}
	if chat == nil {
		return false, apperror.NewNotFound("chat not found")
	}
	if chat.Type != chatDomain.ChatTypeGroup {
		if s.privacy == nil {
			return true, nil
		}
		participants, err := s.chat.ListParticipants(ctx, chatID)
		if err != nil {
			return false, err
		}
		for _, participant := range participants {
			if participant.UserID != userID {
				return s.privacy.CanStartDirectChat(ctx, userID, participant.UserID)
			}
		}
		return false, nil
	}

	group, err := s.repo.FindByChatID(ctx, chatID)
	if err != nil {
		return false, err
	}
	if group == nil {
		return false, apperror.NewNotFound("group not found")
	}
	if !group.OnlyAdminsCanPost {
		return true, nil
	}

	role, isMember, err := s.chat.GetParticipantRole(ctx, chatID, userID)
	if err != nil {
		return false, err
	}
	if !isMember {
		return false, nil
	}
	return role == chatDomain.RoleOwner || role == chatDomain.RoleAdmin, nil
}
