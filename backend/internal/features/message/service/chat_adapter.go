package service

import (
	"context"
	chatdomain "github.com/bharatchat/backend/internal/features/chat/domain"
)

type ChatServiceAuthorizerAdapter struct {
	AuthorizeFn   func(context.Context, string, string) error
	InteractionFn func(context.Context, string, string) error
}

func (a *ChatServiceAuthorizerAdapter) AuthorizeParticipant(ctx context.Context, c, u string) error {
	return a.AuthorizeFn(ctx, c, u)
}

func (a *ChatServiceAuthorizerAdapter) AuthorizeInteraction(ctx context.Context, c, u string) error {
	if a.InteractionFn != nil {
		return a.InteractionFn(ctx, c, u)
	}
	return a.AuthorizeFn(ctx, c, u)
}

type ChatParticipantListerAdapter struct {
	ListParticipantsFn func(context.Context, string) ([]chatdomain.Participant, error)
}

func (a *ChatParticipantListerAdapter) ListOtherParticipantIDs(ctx context.Context, c, exclude string) ([]string, error) {
	p, err := a.ListParticipantsFn(ctx, c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(p))
	for _, v := range p {
		if v.UserID != exclude {
			ids = append(ids, v.UserID)
		}
	}
	return ids, nil
}
