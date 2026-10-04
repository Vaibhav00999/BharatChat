package domain

import "context"

type MessageRepository interface {
	Create(context.Context, Message, []string) (*Message, error)
	ListHistory(context.Context, string, int, *string) ([]Message, error)
	FindByID(context.Context, string) (*Message, error)
	UpdateStatus(context.Context, string, string, DeliveryState) (bool, error)
	MarkAllReadUpTo(context.Context, string, string, string) ([]string, error)
	GetStatusesForMessage(context.Context, string) ([]MessageStatus, error)
}
