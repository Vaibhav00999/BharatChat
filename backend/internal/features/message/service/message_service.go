// internal/features/message/service/message_service.go
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bharatchat/backend/internal/features/message/domain"
	"github.com/bharatchat/backend/pkg/apperror"
)

// ChatAuthorizer is the small slice of chat/service.ChatService that message needs —
// participant authorization and the recipient list — satisfied structurally without
// message importing chat's internal packages beyond this interface's shape.
type ChatAuthorizer interface {
	AuthorizeParticipant(ctx context.Context, chatID, userID string) error
}

// ParticipantLister returns every OTHER active participant of a chat (excluding the
// given user) — used to know who needs a message_status row and a fan-out event.
type ParticipantLister interface {
	ListOtherParticipantIDs(ctx context.Context, chatID, excludingUserID string) ([]string, error)
}

// EventPublisher is implemented by internal/platform/websocket.Hub (structurally) —
// message/service depends on this narrow interface, not the concrete Hub type,
// keeping the dependency arrow pointing from message -> websocket-abstraction,
// not the reverse.
type EventPublisher interface {
	PublishToUser(ctx context.Context, userID string, envelopeType string, payload interface{}) error
}

type GroupPostingAuthorizer interface {
	CanPost(ctx context.Context, chatID, userID string) (bool, error)
}

type ReadReceiptPolicy interface {
	ReadReceiptsEnabled(ctx context.Context, userID string) (bool, error)
}

type MessageService struct {
	repo              domain.MessageRepository
	chatAuth          ChatAuthorizer
	participants      ParticipantLister
	publisher         EventPublisher
	groupPostingAuth  GroupPostingAuthorizer
	readReceiptPolicy ReadReceiptPolicy
	requireEncryption bool
	messagingDisabled bool
}

// SetMessagingEnabled is configured once at startup, before serving requests.
func (s *MessageService) SetMessagingEnabled(enabled bool) {
	s.messagingDisabled = !enabled
}

func (s *MessageService) RequireEncryption(required bool) {
	s.requireEncryption = required
}

func (s *MessageService) UseReadReceiptPolicy(policy ReadReceiptPolicy) {
	s.readReceiptPolicy = policy
}

func NewMessageService(repo domain.MessageRepository, chatAuth ChatAuthorizer, participants ParticipantLister, publisher EventPublisher, groupPostingAuth ...GroupPostingAuthorizer) *MessageService {
	var groupAuth GroupPostingAuthorizer
	if len(groupPostingAuth) > 0 {
		groupAuth = groupPostingAuth[0]
	}
	return &MessageService{repo: repo, chatAuth: chatAuth, participants: participants, publisher: publisher, groupPostingAuth: groupAuth}
}

type SendMessageInput struct {
	ChatID             string
	SenderID           string
	Type               domain.MessageType
	Body               string
	ReplyToMessageID   *string
	ClientGeneratedID  *string
	Ciphertext         []byte
	EncryptionProtocol string
	EncryptionVersion  int
	SenderDeviceID     *string
}

// SentMessageResult carries everything both the REST-equivalent caller and the WS
// handler need: the persisted message plus who else needs to be notified.
type SentMessageResult struct {
	Message          domain.Message
	RecipientUserIDs []string
}

func (s *MessageService) AuthorizeChat(ctx context.Context, chatID, userID string) error {
	if policy, ok := s.chatAuth.(interface {
		AuthorizeInteraction(context.Context, string, string) error
	}); ok {
		return policy.AuthorizeInteraction(ctx, chatID, userID)
	}
	return s.chatAuth.AuthorizeParticipant(ctx, chatID, userID)
}

func (s *MessageService) SendMessage(ctx context.Context, input SendMessageInput) (*SentMessageResult, error) {
	if s.messagingDisabled {
		return nil, apperror.NewForbidden("messaging is not available during prelaunch testing")
	}
	if err := s.AuthorizeChat(ctx, input.ChatID, input.SenderID); err != nil {
		return nil, err
	}
	if s.groupPostingAuth != nil {
		canPost, err := s.groupPostingAuth.CanPost(ctx, input.ChatID, input.SenderID)
		if err != nil {
			return nil, err
		}
		if !canPost {
			return nil, apperror.NewForbidden("only group admins can send messages in this group")
		}
	}

	if input.Type != domain.MessageTypeText {
		return nil, apperror.NewValidation("only text messages are supported in this module")
	}

	encrypted := len(input.Ciphertext) > 0 || input.EncryptionProtocol != "" || input.EncryptionVersion > 0
	trimmedBody := strings.TrimSpace(input.Body)
	if encrypted {
		if trimmedBody != "" || len(input.Ciphertext) < 16 || len(input.Ciphertext) > 65536 || (input.EncryptionProtocol != "signal" && input.EncryptionProtocol != "mls") || input.EncryptionVersion != 1 || input.SenderDeviceID == nil {
			return nil, apperror.NewValidation("invalid encrypted message payload")
		}
	} else {
		if s.requireEncryption {
			return nil, apperror.NewValidation("end-to-end encryption is required")
		}
		if trimmedBody == "" {
			return nil, apperror.NewValidation("message body cannot be empty")
		}
		if len(trimmedBody) > 8000 {
			return nil, apperror.NewValidation("message body is too long")
		}
	}
	if input.ReplyToMessageID != nil {
		repliedTo, err := s.repo.FindByID(ctx, *input.ReplyToMessageID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if repliedTo == nil || repliedTo.ChatID != input.ChatID {
			return nil, apperror.NewValidation("reply target must belong to this chat")
		}
	}

	recipients, err := s.participants.ListOtherParticipantIDs(ctx, input.ChatID, input.SenderID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	var body *string
	if !encrypted {
		body = &trimmedBody
	}
	msg := domain.Message{
		ChatID:             input.ChatID,
		SenderID:           &input.SenderID,
		Type:               input.Type,
		Body:               body,
		ReplyToMessageID:   input.ReplyToMessageID,
		ClientGeneratedID:  input.ClientGeneratedID,
		Ciphertext:         input.Ciphertext,
		EncryptionProtocol: input.EncryptionProtocol,
		EncryptionVersion:  input.EncryptionVersion,
		SenderDeviceID:     input.SenderDeviceID,
	}

	created, err := s.repo.Create(ctx, msg, recipients)
	if errors.Is(err, domain.ErrInteractionBlocked) {
		return nil, apperror.NewForbidden("direct chat is not available")
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}

	// Fan out to every recipient's live connections (any replica, via Redis pub/sub
	// inside the Hub implementation behind EventPublisher). Delivery-state tracking
	// (marking 'delivered') only happens once the recipient's client actually
	// acknowledges receipt over WS — see MarkDelivered below — so this publish is
	// "best effort real-time" layered on top of the REST history as the source of truth.
	for _, recipientID := range recipients {
		if pubErr := s.publisher.PublishToUser(ctx, recipientID, "new_message", toWireMessage(*created)); pubErr != nil {
			// A publish failure must never fail the whole send — the message is
			// already durably persisted; the recipient will still see it via REST
			// history/polling. Only real-time push is degraded.
			continue
		}
	}

	return &SentMessageResult{Message: *created, RecipientUserIDs: recipients}, nil
}

func (s *MessageService) GetHistory(ctx context.Context, chatID, userID string, limit int, beforeMessageID *string) ([]domain.Message, error) {
	if err := s.chatAuth.AuthorizeParticipant(ctx, chatID, userID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	messages, err := s.repo.ListHistory(ctx, chatID, limit, beforeMessageID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return messages, nil
}

// MarkDelivered is called when a recipient's client acknowledges receipt of a
// message over WS. It notifies the ORIGINAL SENDER (not the recipient) that their
// message was delivered — this is the "double tick" the sender sees.
func (s *MessageService) MarkDelivered(ctx context.Context, messageID, recipientUserID string) error {
	changed, err := s.repo.UpdateStatus(ctx, messageID, recipientUserID, domain.StateDelivered)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !changed {
		return nil
	}

	msg, err := s.repo.FindByID(ctx, messageID)
	if err != nil || msg == nil || msg.SenderID == nil {
		return nil // best-effort notification; the status row is already durably updated
	}

	_ = s.publisher.PublishToUser(ctx, *msg.SenderID, "delivery_ack", map[string]interface{}{
		"messageId":       messageID,
		"recipientUserId": recipientUserID,
		"deliveredAt":     time.Now().UTC(),
	})
	return nil
}

// MarkRead marks a single message as read by recipientUserID and notifies the sender.
// Used when a client explicitly acks one message (e.g. it scrolls into view).
func (s *MessageService) MarkRead(ctx context.Context, messageID, recipientUserID string) error {
	changed, err := s.repo.UpdateStatus(ctx, messageID, recipientUserID, domain.StateRead)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !changed {
		return nil
	}
	if !s.readReceiptsEnabled(ctx, recipientUserID) {
		return nil
	}

	msg, err := s.repo.FindByID(ctx, messageID)
	if err != nil || msg == nil || msg.SenderID == nil {
		return nil
	}

	_ = s.publisher.PublishToUser(ctx, *msg.SenderID, "read_ack", map[string]interface{}{
		"messageId":       messageID,
		"recipientUserId": recipientUserID,
		"readAt":          time.Now().UTC(),
	})
	return nil
}

// MarkChatReadUpTo is the bulk equivalent of MarkRead, called once when a user opens
// a chat — it reads every not-yet-read message in one DB round trip and notifies
// each distinct sender exactly once per affected message.
func (s *MessageService) MarkChatReadUpTo(ctx context.Context, chatID, userID, upToMessageID string) error {
	if err := s.chatAuth.AuthorizeParticipant(ctx, chatID, userID); err != nil {
		return err
	}

	changedMessageIDs, err := s.repo.MarkAllReadUpTo(ctx, chatID, userID, upToMessageID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !s.readReceiptsEnabled(ctx, userID) {
		return nil
	}

	for _, messageID := range changedMessageIDs {
		msg, err := s.repo.FindByID(ctx, messageID)
		if err != nil || msg == nil || msg.SenderID == nil {
			continue
		}
		_ = s.publisher.PublishToUser(ctx, *msg.SenderID, "read_ack", map[string]interface{}{
			"messageId":       messageID,
			"recipientUserId": userID,
			"readAt":          time.Now().UTC(),
		})
	}

	return nil
}

func (s *MessageService) readReceiptsEnabled(ctx context.Context, userID string) bool {
	if s.readReceiptPolicy == nil {
		return true
	}
	enabled, err := s.readReceiptPolicy.ReadReceiptsEnabled(ctx, userID)
	return err == nil && enabled
}

// toWireMessage is the single place a persisted domain.Message becomes the JSON
// shape sent over WS — kept here (not in transport) since both the WS handler and
// this service's own fan-out call it, and it must never drift between the two paths.
func toWireMessage(m domain.Message) map[string]interface{} {
	return map[string]interface{}{
		"id":                 m.ID,
		"chatId":             m.ChatID,
		"senderId":           m.SenderID,
		"type":               string(m.Type),
		"body":               m.Body,
		"ciphertext":         m.Ciphertext,
		"encryptionProtocol": m.EncryptionProtocol,
		"encryptionVersion":  m.EncryptionVersion,
		"senderDeviceId":     m.SenderDeviceID,
		"replyToMessageId":   m.ReplyToMessageID,
		"clientGeneratedId":  m.ClientGeneratedID,
		"createdAt":          m.CreatedAt.UTC().Format(time.RFC3339),
		"expiresAt":          m.ExpiresAt,
	}
}
