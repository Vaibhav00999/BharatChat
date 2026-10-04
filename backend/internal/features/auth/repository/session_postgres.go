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

type SessionPostgresRepository struct{ pool *pgxpool.Pool }

func NewSessionPostgresRepository(p *pgxpool.Pool) *SessionPostgresRepository {
	return &SessionPostgresRepository{pool: p}
}
func (r *SessionPostgresRepository) Create(ctx context.Context, userID, deviceID, hash, ip, ua string, expires time.Time) (*domain.Session, error) {
	var s domain.Session
	var ipv interface{}
	if ip != "" {
		ipv = ip
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO sessions(user_id,device_id,refresh_token_hash,ip_address,user_agent,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, userID, deviceID, hash, ipv, ua, expires).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("session_postgres: create: %w", err)
	}
	s.UserID = userID
	s.DeviceID = deviceID
	s.RefreshTokenHash = hash
	s.ExpiresAt = expires
	return &s, nil
}
func (r *SessionPostgresRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	var s domain.Session
	err := r.pool.QueryRow(ctx, `SELECT id,user_id,device_id,refresh_token_hash,expires_at,revoked_at,created_at FROM sessions WHERE refresh_token_hash=$1`, hash).Scan(&s.ID, &s.UserID, &s.DeviceID, &s.RefreshTokenHash, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session_postgres: find: %w", err)
	}
	return &s, nil
}

func (r *SessionPostgresRepository) IsSessionActive(ctx context.Context, sessionID, userID string) (bool, error) {
	var active bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sessions s
			JOIN devices d ON d.id=s.device_id
			WHERE s.id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL
			  AND s.expires_at > now() AND d.is_active=TRUE
		)`, sessionID, userID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("session_postgres: active check: %w", err)
	}
	return active, nil
}
func (r *SessionPostgresRepository) RotateRefreshToken(ctx context.Context, id, hash string, expires time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE sessions SET refresh_token_hash=$2,expires_at=$3 WHERE id=$1 AND revoked_at IS NULL AND expires_at>now()`, id, hash, expires)
	if err != nil {
		return fmt.Errorf("session_postgres: rotate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session_postgres: session is not active")
	}
	return nil
}

// RotateRefreshTokenIfCurrent performs refresh-token rotation as a compare-and-swap.
// Only the request presenting the currently stored token hash can win.
func (r *SessionPostgresRepository) RotateRefreshTokenIfCurrent(ctx context.Context, id, currentHash, newHash string, expires time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE sessions
		SET refresh_token_hash=$3,expires_at=$4
		WHERE id=$1 AND refresh_token_hash=$2 AND revoked_at IS NULL AND expires_at>now()`,
		id, currentHash, newHash, expires)
	if err != nil {
		return false, fmt.Errorf("session_postgres: compare-and-swap rotate: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
func (r *SessionPostgresRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, id)
	return err
}
func (r *SessionPostgresRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	return err
}

type DevicePostgresRepository struct{ pool *pgxpool.Pool }

func NewDevicePostgresRepository(p *pgxpool.Pool) *DevicePostgresRepository {
	return &DevicePostgresRepository{pool: p}
}
func (r *DevicePostgresRepository) Upsert(ctx context.Context, userID string, info domain.DeviceInfo) (string, error) {
	if info.InstallationID != nil && *info.InstallationID != "" {
		var id string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO devices(user_id,platform,device_name,push_token,app_version,installation_id)
			VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(user_id,installation_id) DO UPDATE SET
				platform=EXCLUDED.platform,
				device_name=EXCLUDED.device_name,
				push_token=EXCLUDED.push_token,
				app_version=EXCLUDED.app_version,
				last_active_at=now(),
				is_active=TRUE
			RETURNING id`, userID, string(info.Platform), info.DeviceName, info.PushToken, info.AppVersion, *info.InstallationID).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("device_postgres: upsert installation: %w", err)
		}
		return id, nil
	}
	if info.PushToken != nil && *info.PushToken != "" {
		var id string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO devices(user_id,platform,device_name,push_token,app_version)
			VALUES($1,$2,$3,$4,$5)
			ON CONFLICT(push_token) WHERE push_token IS NOT NULL DO UPDATE SET
				platform=EXCLUDED.platform,
				device_name=EXCLUDED.device_name,
				app_version=EXCLUDED.app_version,
				last_active_at=now(),
				is_active=TRUE
			WHERE devices.user_id=EXCLUDED.user_id
			RETURNING id`, userID, string(info.Platform), info.DeviceName, *info.PushToken, info.AppVersion).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("device_postgres: push token belongs to another account")
		}
		if err != nil {
			return "", fmt.Errorf("device_postgres: upsert: %w", err)
		}
		return id, nil
	}
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO devices(user_id,platform,device_name,app_version) VALUES($1,$2,$3,$4) RETURNING id`, userID, string(info.Platform), info.DeviceName, info.AppVersion).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("device_postgres: insert: %w", err)
	}
	return id, err
}
