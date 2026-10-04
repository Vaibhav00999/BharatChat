package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/bharatchat/backend/pkg/apperror"
	"github.com/google/uuid"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9]{3,30}$`)

func (s *UserService) safetyRepository() (domain.SafetyRepository, error) {
	r, ok := s.repo.(domain.SafetyRepository)
	if !ok {
		return nil, apperror.NewInternal(errors.New("user safety repository unavailable"))
	}
	return r, nil
}

func (s *UserService) ResolveUsername(ctx context.Context, viewer, name string) (*domain.Person, error) {
	name = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "@"))
	if !usernamePattern.MatchString(name) {
		return nil, apperror.NewValidation("enter a username with 3-30 letters or digits")
	}
	r, err := s.safetyRepository()
	if err != nil {
		return nil, err
	}
	p, err := r.ResolveUsername(ctx, viewer, name)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if p == nil {
		return nil, apperror.NewNotFound("user not found")
	}
	return p, nil
}

func (s *UserService) ListBlocked(ctx context.Context, viewer, after string) ([]domain.Person, string, error) {
	if after != "" {
		if _, err := uuid.Parse(after); err != nil {
			return nil, "", apperror.NewValidation("invalid cursor")
		}
	}
	r, err := s.safetyRepository()
	if err != nil {
		return nil, "", err
	}
	people, err := r.ListBlocked(ctx, viewer, after, 51)
	if err != nil {
		return nil, "", apperror.NewInternal(err)
	}
	var next string
	if len(people) > 50 {
		people = people[:50]
		next = people[49].ID
	}
	return people, next, nil
}

func validateSafetyTarget(viewer, target string) error {
	id, err := uuid.Parse(target)
	if err != nil || id == uuid.Nil {
		return apperror.NewValidation("invalid user ID")
	}
	if id.String() == viewer {
		return apperror.NewValidation("choose another user")
	}
	return nil
}

func safetyError(err error) error {
	if errors.Is(err, domain.ErrSafetyTargetNotFound) {
		return apperror.NewNotFound("user not found")
	}
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *UserService) SetBlocked(ctx context.Context, viewer, target string, blocked bool) error {
	if err := validateSafetyTarget(viewer, target); err != nil {
		return err
	}
	r, err := s.safetyRepository()
	if err != nil {
		return err
	}
	return safetyError(r.SetBlocked(ctx, viewer, target, blocked))
}

func (s *UserService) ReportUser(ctx context.Context, viewer, target, reason, details string) (string, error) {
	if err := validateSafetyTarget(viewer, target); err != nil {
		return "", err
	}
	switch reason {
	case "spam", "harassment", "nudity", "violence", "fraud", "other":
	default:
		return "", apperror.NewValidation("invalid report reason")
	}
	details = strings.TrimSpace(details)
	if utf8.RuneCountInString(details) > 1000 {
		return "", apperror.NewValidation("report details are too long")
	}
	r, err := s.safetyRepository()
	if err != nil {
		return "", err
	}
	id, err := r.CreateReport(ctx, viewer, target, reason, details)
	return id, safetyError(err)
}
