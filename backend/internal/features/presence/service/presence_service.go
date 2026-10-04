package service

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

const onlineUserKeyPrefix = "presence:online:"
const onlineTTL = 90 * time.Second

type ParticipantLister interface {
	ListOtherParticipantIDs(context.Context, string, string) ([]string, error)
}
type TypingPrivacyPolicy interface {
	TypingIndicatorsEnabled(context.Context, string) (bool, error)
}
type EventPublisher interface {
	PublishToUser(context.Context, string, string, interface{}) error
}
type PresenceService struct {
	rdb          *redis.Client
	publisher    EventPublisher
	participants ParticipantLister
	privacy      TypingPrivacyPolicy
}

func (s *PresenceService) UseTypingPrivacyPolicy(policy TypingPrivacyPolicy) {
	s.privacy = policy
}

func NewPresenceService(r *redis.Client, p EventPublisher, l ParticipantLister) *PresenceService {
	return &PresenceService{rdb: r, publisher: p, participants: l}
}
func (s *PresenceService) MarkOnline(ctx context.Context, u string) error {
	return s.rdb.Set(ctx, onlineUserKeyPrefix+u, "1", onlineTTL).Err()
}
func (s *PresenceService) MarkOffline(ctx context.Context, u string) error {
	return s.rdb.Del(ctx, onlineUserKeyPrefix+u).Err()
}
func (s *PresenceService) IsOnline(ctx context.Context, u string) (bool, error) {
	n, err := s.rdb.Exists(ctx, onlineUserKeyPrefix+u).Result()
	if err != nil {
		return false, fmt.Errorf("presence: %w", err)
	}
	return n > 0, nil
}
func (s *PresenceService) ListOtherParticipantIDs(ctx context.Context, c, u string) ([]string, error) {
	return s.participants.ListOtherParticipantIDs(ctx, c, u)
}
func (s *PresenceService) PublishTypingEvent(ctx context.Context, c, u string, ids []string, typing bool) error {
	if s.privacy != nil {
		enabled, err := s.privacy.TypingIndicatorsEnabled(ctx, u)
		if err != nil || !enabled {
			return nil
		}
	}
	payload := map[string]interface{}{"chatId": c, "fromUserId": u, "isTyping": typing}
	for _, id := range ids {
		_ = s.publisher.PublishToUser(ctx, id, "typing_update", payload)
	}
	return nil
}
