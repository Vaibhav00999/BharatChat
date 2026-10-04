package domain

import "context"

type GroupRepository interface {
	Create(context.Context, GroupInfo) (*GroupInfo, error)
	FindByChatID(context.Context, string) (*GroupInfo, error)
	FindByInviteCode(context.Context, string) (*GroupInfo, error)
	UpdateInfo(context.Context, string, *string, *string, *string) (*GroupInfo, error)
	SetOnlyAdminsCanPost(context.Context, string, bool) error
	SetOnlyAdminsCanEditInfo(context.Context, string, bool) error
	RegenerateInviteCode(context.Context, string) (string, error)
	SetInviteCodeEnabled(context.Context, string, bool) error
	ListMembers(context.Context, string, string) ([]MemberView, error)
}
