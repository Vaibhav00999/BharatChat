package transport

import (
	"github.com/bharatchat/backend/internal/features/message/domain"
	"time"
)

type MessageResponse struct {
	ID                   string     `json:"id"`
	ChatID               string     `json:"chatId"`
	SenderID             *string    `json:"senderId"`
	Type                 string     `json:"type"`
	Body                 *string    `json:"body"`
	Ciphertext           []byte     `json:"ciphertext,omitempty"`
	EncryptionProtocol   string     `json:"encryptionProtocol,omitempty"`
	EncryptionVersion    int        `json:"encryptionVersion,omitempty"`
	SenderDeviceID       *string    `json:"senderDeviceId,omitempty"`
	ReplyToMessageID     *string    `json:"replyToMessageId,omitempty"`
	IsEdited             bool       `json:"isEdited"`
	IsDeletedForEveryone bool       `json:"isDeletedForEveryone"`
	ClientGeneratedID    *string    `json:"clientGeneratedId,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	ExpiresAt            *time.Time `json:"expiresAt,omitempty"`
}

func ToMessageResponse(m domain.Message) MessageResponse {
	return MessageResponse{ID: m.ID, ChatID: m.ChatID, SenderID: m.SenderID, Type: string(m.Type), Body: m.Body, Ciphertext: m.Ciphertext, EncryptionProtocol: m.EncryptionProtocol, EncryptionVersion: m.EncryptionVersion, SenderDeviceID: m.SenderDeviceID, ReplyToMessageID: m.ReplyToMessageID, IsEdited: m.IsEdited, IsDeletedForEveryone: m.IsDeletedForEveryone, ClientGeneratedID: m.ClientGeneratedID, CreatedAt: m.CreatedAt, ExpiresAt: m.ExpiresAt}
}
