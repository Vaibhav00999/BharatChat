package repository

import (
	"context"
	"errors"

	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/jackc/pgx/v5"
)

func (r *UserPostgresRepository) ResolveUsername(ctx context.Context, viewer, username string) (*domain.Person, error) {
	var p domain.Person
	err := r.pool.QueryRow(ctx, `SELECT u.id, u.username, u.display_name
		FROM users u WHERE u.username = $2 AND u.deleted_at IS NULL AND u.id <> $1
		AND NOT EXISTS (SELECT 1 FROM blocked_users b WHERE
		(b.blocker_id = $1 AND b.blocked_id = u.id) OR (b.blocker_id = u.id AND b.blocked_id = $1))`, viewer, username).
		Scan(&p.ID, &p.Username, &p.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func (r *UserPostgresRepository) ListBlocked(ctx context.Context, viewer, after string, limit int) ([]domain.Person, error) {
	rows, err := r.pool.Query(ctx, `SELECT u.id, u.username, u.display_name FROM blocked_users b
		JOIN users u ON u.id = b.blocked_id WHERE b.blocker_id = $1 AND u.deleted_at IS NULL
		AND (NULLIF($2, '')::uuid IS NULL OR u.id > NULLIF($2, '')::uuid) ORDER BY u.id LIMIT $3`, viewer, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	people := make([]domain.Person, 0)
	for rows.Next() {
		var p domain.Person
		if err := rows.Scan(&p.ID, &p.Username, &p.DisplayName); err != nil {
			return nil, err
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

func (r *UserPostgresRepository) SetBlocked(ctx context.Context, viewer, target string, blocked bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// The same pair lock is held by direct-message persistence so a completed
	// block cannot race with a subsequent insert into an existing conversation.
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('direct-chat:' ||
		LEAST($1::uuid::text, $2::uuid::text) || ':' || GREATEST($1::uuid::text, $2::uuid::text), 0))`, viewer, target)
	if err != nil {
		return err
	}
	if blocked {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, target).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return domain.ErrSafetyTargetNotFound
		}
		_, err = tx.Exec(ctx, `INSERT INTO blocked_users (blocker_id, blocked_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, viewer, target)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM blocked_users WHERE blocker_id=$1 AND blocked_id=$2`, viewer, target)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserPostgresRepository) CreateReport(ctx context.Context, viewer, target, reason, details string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO reports (reporter_id, reported_user_id, reason, details)
		SELECT $1, u.id, $3, NULLIF($4,'') FROM users u WHERE u.id=$2 AND u.deleted_at IS NULL RETURNING id`, viewer, target, reason, details).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrSafetyTargetNotFound
	}
	return id, err
}
