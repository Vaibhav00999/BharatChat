package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/bharatchat/backend/internal/features/auth/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type OTPPostgresRepository struct{ pool *pgxpool.Pool }

func NewOTPPostgresRepository(pool *pgxpool.Pool) *OTPPostgresRepository {
	return &OTPPostgresRepository{pool: pool}
}
func (r *OTPPostgresRepository) Create(ctx context.Context, phone, hash string, purpose domain.OTPPurpose, ttl time.Duration, max int) (*domain.OTPChallenge, error) {
	var c domain.OTPChallenge
	err := r.pool.QueryRow(ctx, `INSERT INTO otp_challenges(phone_number,otp_hash,purpose,max_attempts,expires_at) VALUES($1,$2,$3,$4,now()+$5::interval) RETURNING id,attempt_count,expires_at,created_at`, phone, hash, string(purpose), max, fmt.Sprintf("%f seconds", ttl.Seconds())).Scan(&c.ID, &c.AttemptCount, &c.ExpiresAt, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("otp_postgres: create: %w", err)
	}
	c.PhoneNumber = phone
	c.OTPHash = hash
	c.Purpose = purpose
	c.MaxAttempts = max
	return &c, nil
}
func (r *OTPPostgresRepository) FindLatestActive(ctx context.Context, phone string, purpose domain.OTPPurpose) (*domain.OTPChallenge, error) {
	var c domain.OTPChallenge
	err := r.pool.QueryRow(ctx, `SELECT id,otp_hash,attempt_count,max_attempts,expires_at,consumed_at,created_at FROM otp_challenges WHERE phone_number=$1 AND purpose=$2 AND consumed_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 1`, phone, string(purpose)).Scan(&c.ID, &c.OTPHash, &c.AttemptCount, &c.MaxAttempts, &c.ExpiresAt, &c.ConsumedAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("otp_postgres: find: %w", err)
	}
	c.PhoneNumber = phone
	c.Purpose = purpose
	return &c, nil
}
func (r *OTPPostgresRepository) IncrementAttempt(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE otp_challenges SET attempt_count=attempt_count+1 WHERE id=$1 AND consumed_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("otp_postgres: increment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("otp_postgres: challenge not active")
	}
	return nil
}
func (r *OTPPostgresRepository) MarkConsumed(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE otp_challenges
		SET consumed_at=now()
		WHERE id=$1
		  AND consumed_at IS NULL
		  AND expires_at>now()
		  AND attempt_count<max_attempts`, id)
	if err != nil {
		return fmt.Errorf("otp_postgres: consume: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOTPChallengeInactive
	}
	return nil
}
