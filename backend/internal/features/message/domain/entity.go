package domain

import "time"

type MessageType string

const (
	MessageTypeText      MessageType = "text"
	MessageTypeImage     MessageType = "image"
	MessageTypeVideo     MessageType = "video"
	MessageTypeDocument  MessageType = "document"
	MessageTypeAudio     MessageType = "audio"
	MessageTypeVoiceNote MessageType = "voice_note"
	MessageTypeSystem    MessageType = "system"
)

type DeliveryState string

const (
	StateSent      DeliveryState = "sent"
	StateDelivered DeliveryState = "delivered"
	StateRead      DeliveryState = "read"
)

type Message struct {
	ID, ChatID                                     string
	SenderID                                       *string
	Type                                           MessageType
	Body, ReplyToMessageID, ForwardedFromMessageID *string
	Ciphertext                                     []byte
	EncryptionProtocol                             string
	EncryptionVersion                              int
	SenderDeviceID                                 *string
	ForwardCount                                   int
	IsEdited                                       bool
	EditedAt                                       *time.Time
	IsDeletedForEveryone                           bool
	DeletedAt                                      *time.Time
	ClientGeneratedID                              *string
	ExpiresAt                                      *time.Time
	CreatedAt, UpdatedAt                           time.Time
}
type MessageStatus struct {
	MessageID, UserID   string
	State               DeliveryState
	DeliveredAt, ReadAt *time.Time
}
