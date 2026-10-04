package websocket

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"strings"
	"sync"
)

const redisUserChannelPrefix = "ws:user:"

type sendable interface{ Send(*Envelope) }
type Hub struct {
	mu          sync.RWMutex
	connections map[string]map[*Client]struct{}
	recorders   map[string][]sendable
	rdb         *redis.Client
	log         zerolog.Logger
	pubsub      *redis.PubSub
}

func NewHub(rdb *redis.Client, log zerolog.Logger) *Hub {
	return &Hub{connections: map[string]map[*Client]struct{}{}, recorders: map[string][]sendable{}, rdb: rdb, log: log}
}
func (h *Hub) Register(userID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.connections[userID] == nil {
		h.connections[userID] = map[*Client]struct{}{}
	}
	h.connections[userID][c] = struct{}{}
}
func (h *Hub) Unregister(userID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections[userID], c)
	if len(h.connections[userID]) == 0 {
		delete(h.connections, userID)
	}
}
func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections[userID]) > 0
}
func (h *Hub) localSend(userID string, env *Envelope) {
	h.mu.RLock()
	targets := make([]sendable, 0, len(h.connections[userID])+len(h.recorders[userID]))
	for c := range h.connections[userID] {
		targets = append(targets, c)
	}
	targets = append(targets, h.recorders[userID]...)
	h.mu.RUnlock()
	for _, t := range targets {
		t.Send(env)
	}
}
func (h *Hub) PublishToUser(ctx context.Context, userID string, env *Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return h.rdb.Publish(ctx, redisUserChannelPrefix+userID, raw).Err()
}
func (h *Hub) StartSubscriber(ctx context.Context) error {
	h.pubsub = h.rdb.PSubscribe(ctx, redisUserChannelPrefix+"*")
	if _, err := h.pubsub.Receive(ctx); err != nil {
		_ = h.pubsub.Close()
		return err
	}
	go func() {
		ch := h.pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				_ = h.pubsub.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				userID := strings.TrimPrefix(msg.Channel, redisUserChannelPrefix)
				var env Envelope
				if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
					h.log.Error().Err(err).Msg("hub: invalid pubsub envelope")
					continue
				}
				h.localSend(userID, &env)
			}
		}
	}()
	return nil
}
