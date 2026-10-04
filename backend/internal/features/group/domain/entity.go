package domain

import "time"

type GroupInfo struct {
	ChatID, Name                                                string
	Description, IconURL, InviteCode                            *string
	InviteCodeEnabled, OnlyAdminsCanPost, OnlyAdminsCanEditInfo bool
	MaxMembers                                                  int
	CreatedAt, UpdatedAt                                        time.Time
}
type MemberView struct {
	UserID, DisplayName string
	AvatarURL           *string
	Role                string
	IsOnline            bool
	JoinedAt            time.Time
}
