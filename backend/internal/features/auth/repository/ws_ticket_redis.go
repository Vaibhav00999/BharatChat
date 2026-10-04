package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bharatchat/backend/internal/middleware"
	"github.com/redis/go-redis/v9"
)

const webSocketTicketTTL = 30 * time.Second

type WebSocketTicketRedis struct {
	rdb *redis.Client
}

func NewWebSocketTicketRedis(rdb *redis.Client) *WebSocketTicketRedis {
	return &WebSocketTicketRedis{rdb: rdb}
}

func (s *WebSocketTicketRedis) Issue(ctx context.Context, userID, sessionID string) (string, time.Duration, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", 0, fmt.Errorf("ws_ticket: generate: %w", err)
	}
	ticket := base64.RawURLEncoding.EncodeToString(random)
	payload, err := json.Marshal(middleware.AuthenticatedSession{UserID: userID, SessionID: sessionID})
	if err != nil {
		return "", 0, fmt.Errorf("ws_ticket: encode: %w", err)
	}
	if err := s.rdb.Set(ctx, "ws:ticket:"+ticket, payload, webSocketTicketTTL).Err(); err != nil {
		return "", 0, fmt.Errorf("ws_ticket: store: %w", err)
	}
	return ticket, webSocketTicketTTL, nil
}

// Consume atomically reads and deletes a ticket. A captured URL cannot be
// replayed to establish a second WebSocket connection.
func (s *WebSocketTicketRedis) Consume(ctx context.Context, ticket string) (middleware.AuthenticatedSession, bool, error) {
	if len(ticket) != 43 {
		return middleware.AuthenticatedSession{}, false, nil
	}
	payload, err := s.rdb.GetDel(ctx, "ws:ticket:"+ticket).Bytes()
	if err == redis.Nil {
		return middleware.AuthenticatedSession{}, false, nil
	}
	if err != nil {
		return middleware.AuthenticatedSession{}, false, fmt.Errorf("ws_ticket: consume: %w", err)
	}
	var session middleware.AuthenticatedSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return middleware.AuthenticatedSession{}, false, fmt.Errorf("ws_ticket: decode: %w", err)
	}
	return session, true, nil
}
