package service_test

import (
	"context"
	"testing"

	"github.com/bharatchat/backend/internal/features/presence/service"
	"github.com/stretchr/testify/require"
)

type typingPolicy struct{ enabled bool }

func (p *typingPolicy) TypingIndicatorsEnabled(context.Context, string) (bool, error) {
	return p.enabled, nil
}

type presencePublisher struct{ count int }

func (p *presencePublisher) PublishToUser(context.Context, string, string, interface{}) error {
	p.count++
	return nil
}

func TestTypingIndicatorPrivacyIsEnforcedBeforePublishing(t *testing.T) {
	publisher := &presencePublisher{}
	svc := service.NewPresenceService(nil, publisher, nil)
	svc.UseTypingPrivacyPolicy(&typingPolicy{enabled: false})

	require.NoError(t, svc.PublishTypingEvent(context.Background(), "chat-1", "user-a", []string{"user-b"}, true))
	require.Zero(t, publisher.count)

	svc.UseTypingPrivacyPolicy(&typingPolicy{enabled: true})
	require.NoError(t, svc.PublishTypingEvent(context.Background(), "chat-1", "user-a", []string{"user-b"}, true))
	require.Equal(t, 1, publisher.count)
}
