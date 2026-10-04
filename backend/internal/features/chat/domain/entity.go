package domain

import "time"

type ChatType string

const (
	ChatTypeDirect ChatType = "direct"
	ChatTypeGroup  ChatType = "group"
)

type MemberRole string

const (
	RoleOwner  MemberRole = "owner"
	RoleAdmin  MemberRole = "admin"
	RoleMember MemberRole = "member"
)

type Chat struct {
	ID                                   string
	Type                                 ChatType
	CreatedBy, LastMessageID             *string
	LastActivityAt, CreatedAt, UpdatedAt time.Time
}
type Participant struct {
	ID, ChatID, UserID            string
	Role                          MemberRole
	IsMuted, IsPinned, IsArchived bool
	LastReadMessageID             *string
	JoinedAt                      time.Time
	LeftAt                        *time.Time
}
type ChatSummary struct {
	Chat                                       Chat
	PeerUserID, PeerDisplayName                string
	PeerAvatarURL                              *string
	PeerIsOnline                               bool
	GroupName, GroupIconURL                    *string
	LastMessageBody                            *string
	LastMessageType                            string
	LastMessageSenderID, LastMessageSenderName *string
	UnreadCount                                int
	IsMuted, IsPinned                          bool
}
