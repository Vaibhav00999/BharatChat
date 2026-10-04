package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const privacyCleanupInterval = time.Minute

// StartPrivacyCleanup removes content and authentication metadata after their
// declared retention period and anonymizes old prekey claims. Work is bounded so
// cleanup cannot monopolize the database after downtime.
func StartPrivacyCleanup(ctx context.Context, pool *pgxpool.Pool, log zerolog.Logger) {
	go func() {
		ticker := time.NewTicker(privacyCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := runPrivacyCleanup(ctx, pool); err != nil {
					log.Error().Err(err).Msg("privacy retention cleanup failed")
				}
			}
		}
	}()
}

func runPrivacyCleanup(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`DELETE FROM messages WHERE id IN (
			SELECT id FROM messages WHERE expires_at <= now() ORDER BY expires_at LIMIT 1000
		)`,
		`DELETE FROM otp_challenges WHERE id IN (
			SELECT id FROM otp_challenges
			WHERE expires_at < now() - interval '1 day' OR consumed_at < now() - interval '1 day'
			ORDER BY created_at LIMIT 1000
		)`,
		`UPDATE one_time_pre_keys
		 SET claimed_by_user_id=NULL,claimed_by_device_id=NULL
		 WHERE (device_id,key_id) IN (
			SELECT device_id,key_id FROM one_time_pre_keys
			WHERE claimed_at < now() - interval '7 days'
			  AND (claimed_by_user_id IS NOT NULL OR claimed_by_device_id IS NOT NULL)
			LIMIT 1000
		)`,
		`DELETE FROM sessions WHERE id IN (
			SELECT id FROM sessions
			WHERE (revoked_at IS NOT NULL AND revoked_at < now() - interval '30 days')
			   OR expires_at < now() - interval '30 days'
			ORDER BY created_at LIMIT 1000
		)`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
