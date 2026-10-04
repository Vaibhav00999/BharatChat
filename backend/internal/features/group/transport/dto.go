package transport

import (
	"github.com/bharatchat/backend/internal/features/group/domain"
	"time"
)

type CreateGroupRequest struct {
	Name          string   `json:"name" validate:"required,min=1,max=120"`
	Description   string   `json:"description" validate:"max=500"`
	MemberUserIDs []string `json:"memberUserIds" validate:"required,min=1,dive,uuid4"`
}
type GroupInfoResponse struct {
	ChatID                string    `json:"chatId"`
	Name                  string    `json:"name"`
	Description           *string   `json:"description,omitempty"`
	IconURL               *string   `json:"iconUrl,omitempty"`
	InviteCode            *string   `json:"inviteCode,omitempty"`
	InviteCodeEnabled     bool      `json:"inviteCodeEnabled"`
	OnlyAdminsCanPost     bool      `json:"onlyAdminsCanPost"`
	OnlyAdminsCanEditInfo bool      `json:"onlyAdminsCanEditInfo"`
	MaxMembers            int       `json:"maxMembers"`
	CreatedAt             time.Time `json:"createdAt"`
}

func ToGroupInfoResponse(g *domain.GroupInfo) GroupInfoResponse {
	return GroupInfoResponse{ChatID: g.ChatID, Name: g.Name, Description: g.Description, IconURL: g.IconURL, InviteCode: g.InviteCode, InviteCodeEnabled: g.InviteCodeEnabled, OnlyAdminsCanPost: g.OnlyAdminsCanPost, OnlyAdminsCanEditInfo: g.OnlyAdminsCanEditInfo, MaxMembers: g.MaxMembers, CreatedAt: g.CreatedAt}
}

type MemberResponse struct {
	UserID      string    `json:"userId"`
	DisplayName string    `json:"displayName"`
	AvatarURL   *string   `json:"avatarUrl,omitempty"`
	Role        string    `json:"role"`
	IsOnline    bool      `json:"isOnline"`
	JoinedAt    time.Time `json:"joinedAt"`
}

func ToMemberResponse(m domain.MemberView) MemberResponse {
	return MemberResponse{UserID: m.UserID, DisplayName: m.DisplayName, AvatarURL: m.AvatarURL, Role: m.Role, IsOnline: m.IsOnline, JoinedAt: m.JoinedAt}
}

type UpdateGroupInfoRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=120"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=500"`
	IconURL     *string `json:"iconUrl,omitempty" validate:"omitempty,url"`
}
type SetGroupFlagRequest struct {
	Value bool `json:"value"`
}
type AddMemberRequest struct {
	UserID string `json:"userId" validate:"required,uuid4"`
}
type SetAdminRequest struct {
	IsAdmin bool `json:"isAdmin"`
}
type TransferOwnershipRequest struct {
	NewOwnerUserID string `json:"newOwnerUserId" validate:"required,uuid4"`
}
type JoinViaInviteRequest struct {
	InviteCode string `json:"inviteCode" validate:"required"`
}
type InviteCodeResponse struct {
	InviteCode string `json:"inviteCode"`
}
