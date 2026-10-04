package domain

import "context"

type ChatRepository interface {
	FindOrCreateDirectChat(context.Context, string, string) (*Chat, error)
	ListChatSummariesForUser(context.Context, string) ([]ChatSummary, error)
	FindByID(context.Context, string) (*Chat, error)
	ListParticipants(context.Context, string) ([]Participant, error)
	IsParticipant(context.Context, string, string) (bool, error)
	UpdateLastRead(context.Context, string, string, string) error
	SetMuted(context.Context, string, string, bool) error
	SetPinned(context.Context, string, string, bool) error
	SetArchived(context.Context, string, string, bool) error
	CreateGroupChat(context.Context, string, []string) (*Chat, error)
	DeleteGroupChat(context.Context, string) error
	GetParticipantRole(context.Context, string, string) (MemberRole, bool, error)
	UpdateParticipantRole(context.Context, string, string, MemberRole) error
	AddParticipant(context.Context, string, string, MemberRole) error
	RemoveParticipant(context.Context, string, string) error
	CountParticipants(context.Context, string) (int, error)
}
