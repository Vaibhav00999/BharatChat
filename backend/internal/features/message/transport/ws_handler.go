// internal/features/message/transport/ws_handler.go
package transport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bharatchat/backend/internal/features/message/domain"
	"github.com/bharatchat/backend/internal/features/message/service"
	"github.com/bharatchat/backend/internal/middleware"
	wsplatform "github.com/bharatchat/backend/internal/platform/websocket"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// TypingBroadcaster is the narrow surface the WS handler needs from the presence
// feature to forward typing events to the OTHER participant(s) of a chat — kept
// separate from EventPublisher (which targets a single userID) since typing needs
// to resolve "who else is in this chat" first.
type TypingBroadcaster interface {
	ListOtherParticipantIDs(ctx context.Context, chatID, excludingUserID string) ([]string, error)
	PublishTypingEvent(ctx context.Context, chatID, fromUserID string, recipientUserIDs []string, isTyping bool) error
}

type MessageWSHandler struct {
	hub            *wsplatform.Hub
	msgService     *service.MessageService
	typing         TypingBroadcaster
	log            zerolog.Logger
	upgrader       websocket.Upgrader
	deviceResolver SessionDeviceResolver
}

type SessionDeviceResolver interface {
	ResolveDeviceID(ctx context.Context, sessionID, userID string) (string, error)
}

func (h *MessageWSHandler) WithSessionDeviceResolver(resolver SessionDeviceResolver) *MessageWSHandler {
	h.deviceResolver = resolver
	return h
}

func NewMessageWSHandler(hub *wsplatform.Hub, msgService *service.MessageService, typing TypingBroadcaster, log zerolog.Logger, allowedOrigins ...string) *MessageWSHandler {
	allowAll := len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*")
	return &MessageWSHandler{
		hub: hub, msgService: msgService, typing: typing, log: log,
		upgrader: websocket.Upgrader{
			ReadBufferSize: 1024, WriteBufferSize: 1024,
			Subprotocols: []string{"bharatchat.v1"},
			CheckOrigin: func(request *http.Request) bool {
				origin := request.Header.Get("Origin")
				if origin == "" || allowAll {
					return true
				}
				for _, allowed := range allowedOrigins {
					if strings.EqualFold(origin, allowed) {
						return true
					}
				}
				return false
			},
		},
	}
}

func (h *MessageWSHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/ws", h.HandleUpgrade)
}

// HandleUpgrade upgrades an authenticated HTTP request (RequireAuth has already
// run and populated userID in context) to a WebSocket connection, registers it
// with the Hub, and blocks for the connection's lifetime.
func (h *MessageWSHandler) HandleUpgrade(c *gin.Context) {
	userID := middleware.UserIDFromContext(c).String()
	deviceID := ""
	if h.deviceResolver != nil {
		var err error
		deviceID, err = h.deviceResolver.ResolveDeviceID(c.Request.Context(), middleware.SessionIDFromContext(c).String(), userID)
		if err != nil || deviceID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "session is no longer active"})
			return
		}
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Error().Err(err).Msg("ws: upgrade failed")
		return
	}

	sessionID := middleware.SessionIDFromContext(c).String()
	client := wsplatform.NewClient(conn, userID, h.hub, h.log, func(ctx context.Context, userID string, env *wsplatform.Envelope) {
		h.handleInboundEvent(ctx, userID, deviceID, env)
	}, func(ctx context.Context) bool {
		if h.deviceResolver == nil {
			return true
		}
		resolved, err := h.deviceResolver.ResolveDeviceID(ctx, sessionID, userID)
		return err == nil && resolved == deviceID
	})
	h.hub.Register(userID, client)

	client.Run(c.Request.Context())
}

// handleInboundEvent is the single dispatch point for every client->server WS frame,
// decoding the envelope's payload only as far as each specific event type requires.
func (h *MessageWSHandler) handleInboundEvent(ctx context.Context, userID, deviceID string, env *wsplatform.Envelope) {
	switch env.Type {
	case wsplatform.EventSendMessage:
		h.handleSendMessage(ctx, userID, deviceID, env)
	case wsplatform.EventMessageDelivered:
		h.handleMessageDelivered(ctx, userID, env)
	case wsplatform.EventMessageRead:
		h.handleMessageRead(ctx, userID, env)
	case wsplatform.EventTypingStart:
		h.handleTyping(ctx, userID, env, true)
	case wsplatform.EventTypingStop:
		h.handleTyping(ctx, userID, env, false)
	default:
		h.log.Warn().Str("type", string(env.Type)).Msg("ws: unrecognized inbound event type")
	}
}

type sendMessagePayload struct {
	ChatID             string  `json:"chatId"`
	Type               string  `json:"type"`
	Body               string  `json:"body"`
	ReplyToMessageID   *string `json:"replyToMessageId,omitempty"`
	ClientGeneratedID  *string `json:"clientGeneratedId,omitempty"`
	Ciphertext         string  `json:"ciphertext,omitempty"`
	EncryptionProtocol string  `json:"encryptionProtocol,omitempty"`
	EncryptionVersion  int     `json:"encryptionVersion,omitempty"`
}

func decodeBase64URL(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func (h *MessageWSHandler) handleSendMessage(ctx context.Context, userID, deviceID string, env *wsplatform.Envelope) {
	var payload sendMessagePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		h.log.Warn().Err(err).Msg("ws: failed to decode send_message payload")
		return
	}
	if _, err := uuid.Parse(payload.ChatID); err != nil {
		h.sendErrorToSender(ctx, userID, env.RequestID, apperror.NewValidation("chatId must be a UUID"))
		return
	}
	if payload.ClientGeneratedID != nil {
		if _, err := uuid.Parse(*payload.ClientGeneratedID); err != nil {
			h.sendErrorToSender(ctx, userID, env.RequestID, apperror.NewValidation("clientGeneratedId must be a UUID"))
			return
		}
	}
	ciphertext, err := decodeBase64URL(payload.Ciphertext)
	if err != nil {
		h.sendErrorToSender(ctx, userID, env.RequestID, apperror.NewValidation("ciphertext must be valid base64url"))
		return
	}
	var senderDeviceID *string
	if deviceID != "" {
		senderDeviceID = &deviceID
	}

	msgType := domain.MessageType(payload.Type)
	if msgType == "" {
		msgType = domain.MessageTypeText
	}

	result, err := h.msgService.SendMessage(ctx, service.SendMessageInput{
		ChatID:             payload.ChatID,
		SenderID:           userID,
		Type:               msgType,
		Body:               payload.Body,
		ReplyToMessageID:   payload.ReplyToMessageID,
		ClientGeneratedID:  payload.ClientGeneratedID,
		Ciphertext:         ciphertext,
		EncryptionProtocol: payload.EncryptionProtocol,
		EncryptionVersion:  payload.EncryptionVersion,
		SenderDeviceID:     senderDeviceID,
	})
	if err != nil {
		if appErr, ok := apperror.As(err); !ok || appErr.HTTPStatus >= http.StatusInternalServerError {
			h.log.Error().Err(err).Str("user_id", userID).Str("chat_id", payload.ChatID).Msg("ws: send message failed")
		}
		h.sendErrorToSender(ctx, userID, env.RequestID, err)
		return
	}

	// Ack directly back to the sender's own connection(s) — this does not need to
	// go through the general publisher since we already have the Hub reference here
	// and the sender is, by definition, the connection that just sent this frame.
	ackEnvelope, marshalErr := wsplatform.NewEnvelope(wsplatform.EventMessageAck, map[string]interface{}{
		"clientGeneratedId": payload.ClientGeneratedID,
		"message":           messageToWirePayload(result.Message),
	})
	if marshalErr == nil {
		ackEnvelope.RequestID = env.RequestID
		_ = h.hub.PublishToUser(ctx, userID, ackEnvelope)
	}
}

type deliveredPayload struct {
	MessageID string `json:"messageId"`
}

func (h *MessageWSHandler) handleMessageDelivered(ctx context.Context, userID string, env *wsplatform.Envelope) {
	var payload deliveredPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		h.log.Warn().Err(err).Msg("ws: failed to decode message_delivered payload")
		return
	}
	if _, err := uuid.Parse(payload.MessageID); err != nil {
		return
	}
	if err := h.msgService.MarkDelivered(ctx, payload.MessageID, userID); err != nil {
		h.log.Warn().Err(err).Str("message_id", payload.MessageID).Msg("ws: mark delivered failed")
	}
}

type readPayload struct {
	MessageID string `json:"messageId"`
}

func (h *MessageWSHandler) handleMessageRead(ctx context.Context, userID string, env *wsplatform.Envelope) {
	var payload readPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		h.log.Warn().Err(err).Msg("ws: failed to decode message_read payload")
		return
	}
	if _, err := uuid.Parse(payload.MessageID); err != nil {
		return
	}
	if err := h.msgService.MarkRead(ctx, payload.MessageID, userID); err != nil {
		h.log.Warn().Err(err).Str("message_id", payload.MessageID).Msg("ws: mark read failed")
	}
}

type typingPayload struct {
	ChatID string `json:"chatId"`
}

func (h *MessageWSHandler) handleTyping(ctx context.Context, userID string, env *wsplatform.Envelope, isTyping bool) {
	var payload typingPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		h.log.Warn().Err(err).Msg("ws: failed to decode typing payload")
		return
	}
	if _, err := uuid.Parse(payload.ChatID); err != nil {
		return
	}
	if err := h.msgService.AuthorizeChat(ctx, payload.ChatID, userID); err != nil {
		h.sendErrorToSender(ctx, userID, env.RequestID, err)
		return
	}

	recipients, err := h.typing.ListOtherParticipantIDs(ctx, payload.ChatID, userID)
	if err != nil {
		h.log.Warn().Err(err).Msg("ws: failed to list recipients for typing event")
		return
	}

	if err := h.typing.PublishTypingEvent(ctx, payload.ChatID, userID, recipients, isTyping); err != nil {
		h.log.Warn().Err(err).Msg("ws: failed to publish typing event")
	}
}

func (h *MessageWSHandler) sendErrorToSender(ctx context.Context, userID, requestID string, err error) {
	message := "an unexpected error occurred"
	code := "INTERNAL_ERROR"
	if appErr, ok := apperror.As(err); ok {
		message = appErr.Message
		code = appErr.Code
	}
	env, marshalErr := wsplatform.NewEnvelope(wsplatform.EventErrorEvent, map[string]string{"code": code, "message": message})
	if marshalErr != nil {
		return
	}
	env.RequestID = requestID
	_ = h.hub.PublishToUser(ctx, userID, env)
}

func messageToWirePayload(m domain.Message) map[string]interface{} {
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
		"createdAt":          m.CreatedAt,
		"expiresAt":          m.ExpiresAt,
	}
}
