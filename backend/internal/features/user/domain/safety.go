package domain

import (
	"context"
	"errors"
)

var ErrSafetyTargetNotFound = errors.New("safety target not found")
var ErrUsernameTaken = errors.New("username already taken")

// Person exposes only the fields needed for exact-username discovery.
type Person struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	DisplayName string  `json:"displayName"`
}

type SafetyRepository interface {
	ResolveUsername(context.Context, string, string) (*Person, error)
	ListBlocked(context.Context, string, string, int) ([]Person, error)
	SetBlocked(context.Context, string, string, bool) error
	CreateReport(context.Context, string, string, string, string) (string, error)
}
