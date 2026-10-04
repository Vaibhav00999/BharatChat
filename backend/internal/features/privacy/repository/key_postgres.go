package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bharatchat/backend/internal/features/privacy/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KeyPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewKeyPostgresRepository(pool *pgxpool.Pool) *KeyPostgresRepository {
	return &KeyPostgresRepository{pool: pool}
}

func (r *KeyPostgresRepository) ResolveDeviceID(ctx context.Context, sessionID, userID string) (string, error) {
	var deviceID string
	err := r.pool.QueryRow(ctx, `
		SELECT s.device_id
		FROM sessions s
		JOIN devices d ON d.id = s.device_id
		WHERE s.id = $1 AND s.user_id = $2 AND s.revoked_at IS NULL
		  AND s.expires_at > now() AND d.is_active = TRUE`, sessionID, userID).Scan(&deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("privacy_postgres: resolve session device: %w", err)
	}
	return deviceID, nil
}

func (r *KeyPostgresRepository) UpsertBundle(ctx context.Context, sessionID, userID string, bundle domain.DeviceKeyBundle) (*domain.DeviceKeyBundle, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: begin key upload: %w", err)
	}
	defer tx.Rollback(ctx)

	var deviceID string
	err = tx.QueryRow(ctx, `
		SELECT s.device_id
		FROM sessions s
		JOIN devices d ON d.id = s.device_id
		WHERE s.id = $1 AND s.user_id = $2 AND s.revoked_at IS NULL
		  AND s.expires_at > now() AND d.is_active = TRUE
		FOR UPDATE OF d`, sessionID, userID).Scan(&deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: resolve upload device: %w", err)
	}

	bundle.DeviceID = deviceID
	bundle.UserID = userID
	err = tx.QueryRow(ctx, `
		INSERT INTO device_key_bundles (
			device_id, user_id, registration_id, identity_agreement_public_key,
			identity_signing_public_key, signed_pre_key_id,
			signed_pre_key_public_key, signed_pre_key_signature, key_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (device_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			registration_id = EXCLUDED.registration_id,
			identity_agreement_public_key = EXCLUDED.identity_agreement_public_key,
			identity_signing_public_key = EXCLUDED.identity_signing_public_key,
			signed_pre_key_id = EXCLUDED.signed_pre_key_id,
			signed_pre_key_public_key = EXCLUDED.signed_pre_key_public_key,
			signed_pre_key_signature = EXCLUDED.signed_pre_key_signature,
			key_version = EXCLUDED.key_version
		RETURNING created_at, updated_at`,
		deviceID, userID, bundle.RegistrationID, bundle.IdentityAgreementPublicKey,
		bundle.IdentitySigningPublicKey, bundle.SignedPreKeyID,
		bundle.SignedPreKeyPublicKey, bundle.SignedPreKeySignature, bundle.KeyVersion,
	).Scan(&bundle.CreatedAt, &bundle.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: upsert key bundle: %w", err)
	}

	// A signed-prekey rotation invalidates unused one-time prekeys from the old
	// bundle. Claimed keys are retained as a short audit trail until device removal.
	if _, err := tx.Exec(ctx, `DELETE FROM one_time_pre_keys WHERE device_id=$1 AND claimed_at IS NULL`, deviceID); err != nil {
		return nil, fmt.Errorf("privacy_postgres: clear old prekeys: %w", err)
	}
	for _, preKey := range bundle.OneTimePreKeys {
		if _, err := tx.Exec(ctx, `
			INSERT INTO one_time_pre_keys (device_id, key_id, public_key)
			VALUES ($1,$2,$3)
			ON CONFLICT (device_id,key_id) DO NOTHING`,
			deviceID, preKey.KeyID, preKey.PublicKey); err != nil {
			return nil, fmt.Errorf("privacy_postgres: insert one-time prekey: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("privacy_postgres: commit key upload: %w", err)
	}
	return &bundle, nil
}

func (r *KeyPostgresRepository) ClaimBundlesForUser(ctx context.Context, requesterSessionID, requesterUserID, targetUserID string) ([]domain.DeviceKeyBundle, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: begin bundle claim: %w", err)
	}
	defer tx.Rollback(ctx)

	var requesterDeviceID string
	err = tx.QueryRow(ctx, `
		SELECT s.device_id
		FROM sessions s
		JOIN devices d ON d.id=s.device_id
		WHERE s.id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL
		  AND s.expires_at > now() AND d.is_active=TRUE`,
		requesterSessionID, requesterUserID).Scan(&requesterDeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: resolve claimant device: %w", err)
	}

	if requesterUserID != targetUserID {
		var sharesChat bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM chat_participants mine
				JOIN chat_participants theirs ON theirs.chat_id = mine.chat_id
				JOIN chats c ON c.id = mine.chat_id
				WHERE mine.user_id=$1 AND mine.left_at IS NULL
				  AND theirs.user_id=$2 AND theirs.left_at IS NULL
				  AND c.deleted_at IS NULL
			)`, requesterUserID, targetUserID).Scan(&sharesChat)
		if err != nil {
			return nil, fmt.Errorf("privacy_postgres: authorize bundle claim: %w", err)
		}
		if !sharesChat {
			return nil, nil
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT b.device_id,b.user_id,b.registration_id,
		       b.identity_agreement_public_key,b.identity_signing_public_key,
		       b.signed_pre_key_id,b.signed_pre_key_public_key,
		       b.signed_pre_key_signature,b.key_version,b.created_at,b.updated_at
		FROM device_key_bundles b
		JOIN devices d ON d.id=b.device_id
		WHERE b.user_id=$1 AND d.is_active=TRUE AND b.device_id <> $2
		ORDER BY b.created_at
		FOR UPDATE OF b`, targetUserID, requesterDeviceID)
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: list target bundles: %w", err)
	}

	var bundles []domain.DeviceKeyBundle
	for rows.Next() {
		var bundle domain.DeviceKeyBundle
		if err := rows.Scan(
			&bundle.DeviceID, &bundle.UserID, &bundle.RegistrationID,
			&bundle.IdentityAgreementPublicKey, &bundle.IdentitySigningPublicKey,
			&bundle.SignedPreKeyID, &bundle.SignedPreKeyPublicKey,
			&bundle.SignedPreKeySignature, &bundle.KeyVersion,
			&bundle.CreatedAt, &bundle.UpdatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("privacy_postgres: scan target bundle: %w", err)
		}
		bundles = append(bundles, bundle)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("privacy_postgres: iterate target bundles: %w", err)
	}
	rows.Close()

	for i := range bundles {
		var preKey domain.OneTimePreKey
		err := tx.QueryRow(ctx, `
			SELECT key_id,public_key
			FROM one_time_pre_keys
			WHERE device_id=$1 AND claimed_by_device_id=$2
			ORDER BY claimed_at DESC
			LIMIT 1`, bundles[i].DeviceID, requesterDeviceID).Scan(&preKey.KeyID, &preKey.PublicKey)
		if err == nil {
			bundles[i].OneTimePreKey = &preKey
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("privacy_postgres: load existing prekey claim: %w", err)
		}

		err = tx.QueryRow(ctx, `
			UPDATE one_time_pre_keys
			SET claimed_at=now(), claimed_by_user_id=$2, claimed_by_device_id=$3
			WHERE (device_id,key_id) = (
				SELECT device_id,key_id FROM one_time_pre_keys
				WHERE device_id=$1 AND claimed_at IS NULL
				ORDER BY key_id
				FOR UPDATE SKIP LOCKED LIMIT 1
			)
			RETURNING key_id,public_key`, bundles[i].DeviceID, requesterUserID, requesterDeviceID).Scan(&preKey.KeyID, &preKey.PublicKey)
		if err == nil {
			bundles[i].OneTimePreKey = &preKey
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("privacy_postgres: claim one-time prekey: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("privacy_postgres: commit bundle claim: %w", err)
	}
	return bundles, nil
}

func (r *KeyPostgresRepository) ListLinkedDevices(ctx context.Context, userID string) ([]domain.LinkedDevice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id,d.platform,d.device_name,d.last_active_at,
		       (b.device_id IS NOT NULL),
		       COUNT(s.id) FILTER (WHERE s.revoked_at IS NULL AND s.expires_at > now())
		FROM devices d
		LEFT JOIN device_key_bundles b ON b.device_id=d.id
		LEFT JOIN sessions s ON s.device_id=d.id
		WHERE d.user_id=$1 AND d.is_active=TRUE
		GROUP BY d.id,b.device_id
		ORDER BY d.last_active_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("privacy_postgres: list linked devices: %w", err)
	}
	defer rows.Close()

	var devices []domain.LinkedDevice
	for rows.Next() {
		var device domain.LinkedDevice
		if err := rows.Scan(&device.DeviceID, &device.Platform, &device.DeviceName, &device.LastActiveAt, &device.HasKeyBundle, &device.ActiveSessions); err != nil {
			return nil, fmt.Errorf("privacy_postgres: scan linked device: %w", err)
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (r *KeyPostgresRepository) RevokeLinkedDevice(ctx context.Context, ownerUserID, deviceID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("privacy_postgres: begin device revoke: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `UPDATE devices SET is_active=FALSE,push_token=NULL WHERE id=$1 AND user_id=$2 AND is_active=TRUE`, deviceID, ownerUserID)
	if err != nil {
		return false, fmt.Errorf("privacy_postgres: deactivate device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE device_id=$1`, deviceID); err != nil {
		return false, fmt.Errorf("privacy_postgres: revoke device sessions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM device_key_bundles WHERE device_id=$1`, deviceID); err != nil {
		return false, fmt.Errorf("privacy_postgres: remove device keys: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("privacy_postgres: commit device revoke: %w", err)
	}
	return true, nil
}
