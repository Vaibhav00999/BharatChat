package transport

import (
	"github.com/bharatchat/backend/internal/features/chat/domain"
	"time"
)

type StartDirectChatRequest struct {
	PeerUserID string `json:"peerUserId" validate:"required,uuid4"`
}
type ChatResponse struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

func ToChatResponse(c *domain.Chat) ChatResponse {
	return ChatResponse{ID: c.ID, Type: string(c.Type), LastActivityAt: c.LastActivityAt, CreatedAt: c.CreatedAt}
}

type ChatSummaryResponse struct {
	ID                    string    `json:"id"`
	Type                  string    `json:"type"`
	PeerUserID            string    `json:"peerUserId,omitempty"`
	PeerDisplayName       string    `json:"peerDisplayName,omitempty"`
	PeerAvatarURL         *string   `json:"peerAvatarUrl,omitempty"`
	PeerIsOnline          bool      `json:"peerIsOnline"`
	GroupName             *string   `json:"groupName,omitempty"`
	GroupIconURL          *string   `json:"groupIconUrl,omitempty"`
	LastMessageBody       *string   `json:"lastMessageBody,omitempty"`
	LastMessageType       *string   `json:"lastMessageType,omitempty"`
	LastMessageSenderID   *string   `json:"lastMessageSenderId,omitempty"`
	LastMessageSenderName *string   `json:"lastMessageSenderName,omitempty"`
	UnreadCount           int       `json:"unreadCount"`
	IsMuted               bool      `json:"isMuted"`
	IsPinned              bool      `json:"isPinned"`
	LastActivityAt        time.Time `json:"lastActivityAt"`
}

func ToChatSummaryResponse(s domain.ChatSummary) ChatSummaryResponse {
	var t *string
	if s.LastMessageType != "" {
		t = &s.LastMessageType
	}
	return ChatSummaryResponse{ID: s.Chat.ID, Type: string(s.Chat.Type), PeerUserID: s.PeerUserID, PeerDisplayName: s.PeerDisplayName, PeerAvatarURL: s.PeerAvatarURL, PeerIsOnline: s.PeerIsOnline, GroupName: s.GroupName, GroupIconURL: s.GroupIconURL, LastMessageBody: s.LastMessageBody, LastMessageType: t, LastMessageSenderID: s.LastMessageSenderID, LastMessageSenderName: s.LastMessageSenderName, UnreadCount: s.UnreadCount, IsMuted: s.IsMuted, IsPinned: s.IsPinned, LastActivityAt: s.Chat.LastActivityAt}
}

type SetChatFlagRequest struct {
	Value bool `json:"value"`
}
