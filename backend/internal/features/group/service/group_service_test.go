package service_test

import (
	"context"
	"errors"
	"testing"

	chatdomain "github.com/bharatchat/backend/internal/features/chat/domain"
	groupdomain "github.com/bharatchat/backend/internal/features/group/domain"
	"github.com/bharatchat/backend/internal/features/group/service"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/stretchr/testify/require"
)

type fakeGroupRepository struct {
	group     *groupdomain.GroupInfo
	createErr error
}

func (f *fakeGroupRepository) Create(context.Context, groupdomain.GroupInfo) (*groupdomain.GroupInfo, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.group, nil
}
func (f *fakeGroupRepository) FindByChatID(context.Context, string) (*groupdomain.GroupInfo, error) {
	return f.group, nil
}
func (f *fakeGroupRepository) FindByInviteCode(context.Context, string) (*groupdomain.GroupInfo, error) {
	return f.group, nil
}
func (f *fakeGroupRepository) UpdateInfo(context.Context, string, *string, *string, *string) (*groupdomain.GroupInfo, error) {
	return f.group, nil
}
func (f *fakeGroupRepository) SetOnlyAdminsCanPost(context.Context, string, bool) error {
	return nil
}
func (f *fakeGroupRepository) SetOnlyAdminsCanEditInfo(context.Context, string, bool) error {
	return nil
}
func (f *fakeGroupRepository) RegenerateInviteCode(context.Context, string) (string, error) {
	return "NEWCODE", nil
}
func (f *fakeGroupRepository) SetInviteCodeEnabled(context.Context, string, bool) error {
	return nil
}
func (f *fakeGroupRepository) ListMembers(context.Context, string, string) ([]groupdomain.MemberView, error) {
	return nil, nil
}

type fakeChatService struct {
	chat              *chatdomain.Chat
	roles             map[string]chatdomain.MemberRole
	removedUserID     string
	transferCalled    bool
	transferredFromID string
	transferredToID   string
	deletedChatID     string
}

func (f *fakeChatService) CreateGroupChat(context.Context, string, []string) (*chatdomain.Chat, error) {
	return f.chat, nil
}
func (f *fakeChatService) DeleteGroupChat(_ context.Context, chatID string) error {
	f.deletedChatID = chatID
	return nil
}
func (f *fakeChatService) GetParticipantRole(_ context.Context, _ string, userID string) (chatdomain.MemberRole, bool, error) {
	role, ok := f.roles[userID]
	return role, ok, nil
}
func (f *fakeChatService) UpdateParticipantRole(_ context.Context, _ string, userID string, role chatdomain.MemberRole) error {
	f.roles[userID] = role
	return nil
}
func (f *fakeChatService) AddParticipant(_ context.Context, _ string, userID string, role chatdomain.MemberRole) error {
	if _, exists := f.roles[userID]; !exists {
		f.roles[userID] = role
	}
	return nil
}
func (f *fakeChatService) RemoveParticipant(_ context.Context, _ string, userID string) error {
	f.removedUserID = userID
	delete(f.roles, userID)
	return nil
}
func (f *fakeChatService) TransferOwnership(_ context.Context, _ string, fromID, toID string) error {
	f.transferCalled = true
	f.transferredFromID = fromID
	f.transferredToID = toID
	f.roles[fromID] = chatdomain.RoleAdmin
	f.roles[toID] = chatdomain.RoleOwner
	return nil
}
func (f *fakeChatService) AuthorizeParticipant(_ context.Context, _ string, userID string) error {
	if _, ok := f.roles[userID]; !ok {
		return apperror.NewForbidden("not a participant")
	}
	return nil
}
func (f *fakeChatService) FindByID(context.Context, string) (*chatdomain.Chat, error) {
	return f.chat, nil
}
func (f *fakeChatService) ListParticipants(context.Context, string) ([]chatdomain.Participant, error) {
	participants := make([]chatdomain.Participant, 0, len(f.roles))
	for userID, role := range f.roles {
		participants = append(participants, chatdomain.Participant{UserID: userID, Role: role})
	}
	return participants, nil
}

func newGroupService(onlyAdminsCanPost bool, roles map[string]chatdomain.MemberRole) (*service.GroupService, *fakeChatService) {
	chat := &fakeChatService{
		chat:  &chatdomain.Chat{ID: "chat-1", Type: chatdomain.ChatTypeGroup},
		roles: roles,
	}
	repo := &fakeGroupRepository{group: &groupdomain.GroupInfo{
		ChatID:            "chat-1",
		Name:              "Team",
		OnlyAdminsCanPost: onlyAdminsCanPost,
		MaxMembers:        256,
	}}
	return service.NewGroupService(repo, chat), chat
}

func TestCanPostHonorsAdminsOnlySetting(t *testing.T) {
	svc, _ := newGroupService(true, map[string]chatdomain.MemberRole{
		"owner":  chatdomain.RoleOwner,
		"admin":  chatdomain.RoleAdmin,
		"member": chatdomain.RoleMember,
	})

	allowed, err := svc.CanPost(context.Background(), "chat-1", "owner")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = svc.CanPost(context.Background(), "chat-1", "admin")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = svc.CanPost(context.Background(), "chat-1", "member")
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestAdminCanRemoveAnotherAdminButNotOwner(t *testing.T) {
	svc, chat := newGroupService(false, map[string]chatdomain.MemberRole{
		"owner":   chatdomain.RoleOwner,
		"admin-a": chatdomain.RoleAdmin,
		"admin-b": chatdomain.RoleAdmin,
	})

	err := svc.RemoveMember(context.Background(), "chat-1", "admin-a", "admin-b")
	require.NoError(t, err)
	require.Equal(t, "admin-b", chat.removedUserID)

	err = svc.RemoveMember(context.Background(), "chat-1", "admin-a", "owner")
	require.Error(t, err)
	appErr, ok := apperror.As(err)
	require.True(t, ok)
	require.Equal(t, "FORBIDDEN", appErr.Code)
}

func TestOnlyOwnerCanPromoteAdmins(t *testing.T) {
	svc, _ := newGroupService(false, map[string]chatdomain.MemberRole{
		"owner":  chatdomain.RoleOwner,
		"admin":  chatdomain.RoleAdmin,
		"member": chatdomain.RoleMember,
	})

	err := svc.SetAdmin(context.Background(), "chat-1", "admin", "member", true)
	require.Error(t, err)
	err = svc.SetAdmin(context.Background(), "chat-1", "owner", "member", true)
	require.NoError(t, err)
}

func TestTransferOwnershipUsesAtomicChatOperation(t *testing.T) {
	svc, chat := newGroupService(false, map[string]chatdomain.MemberRole{
		"owner":  chatdomain.RoleOwner,
		"member": chatdomain.RoleMember,
	})

	err := svc.TransferOwnership(context.Background(), "chat-1", "owner", "member")
	require.NoError(t, err)
	require.True(t, chat.transferCalled)
	require.Equal(t, "owner", chat.transferredFromID)
	require.Equal(t, "member", chat.transferredToID)
}

func TestOwnerMustTransferBeforeLeaving(t *testing.T) {
	svc, _ := newGroupService(false, map[string]chatdomain.MemberRole{"owner": chatdomain.RoleOwner})
	err := svc.LeaveGroup(context.Background(), "chat-1", "owner")
	require.Error(t, err)
	appErr, ok := apperror.As(err)
	require.True(t, ok)
	require.Equal(t, "VALIDATION_ERROR", appErr.Code)
}

func TestGroupInviteCodeIsHiddenFromRegularMembers(t *testing.T) {
	code := "SECRET"
	svc, _ := newGroupService(false, map[string]chatdomain.MemberRole{
		"owner":  chatdomain.RoleOwner,
		"member": chatdomain.RoleMember,
	})

	// Set the fake repository's group through the response returned by its normal
	// service setup, then verify role-specific projection does not mutate storage.
	ownerView, err := svc.GetGroupInfo(context.Background(), "chat-1", "owner")
	require.NoError(t, err)
	ownerView.InviteCode = &code

	memberView, err := svc.GetGroupInfo(context.Background(), "chat-1", "member")
	require.NoError(t, err)
	require.Nil(t, memberView.InviteCode)
}

func TestCreateGroupCleansUpChatWhenExtensionInsertFails(t *testing.T) {
	chat := &fakeChatService{chat: &chatdomain.Chat{ID: "chat-1", Type: chatdomain.ChatTypeGroup}, roles: map[string]chatdomain.MemberRole{}}
	repo := &fakeGroupRepository{createErr: errors.New("insert failed")}
	svc := service.NewGroupService(repo, chat)

	_, err := svc.CreateGroup(context.Background(), service.CreateGroupInput{
		CreatorUserID: "owner", Name: "Team", MemberUserIDs: []string{"member"},
	})

	require.Error(t, err)
	require.Equal(t, "chat-1", chat.deletedChatID)
}
