package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserPostgresRepository struct{ pool *pgxpool.Pool }

func NewUserPostgresRepository(p *pgxpool.Pool) *UserPostgresRepository {
	return &UserPostgresRepository{pool: p}
}

const userColumns = `id,phone_number,country_code,username,display_name,avatar_url,about,is_verified,last_seen_at,is_online,privacy_last_seen,privacy_avatar,privacy_about,privacy_read_receipts,privacy_phone,discoverable_by_phone,allow_group_adds,share_typing_indicators,security_notifications,created_at,updated_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.PhoneNumber, &u.CountryCode, &u.Username, &u.DisplayName, &u.AvatarURL, &u.About, &u.IsVerified, &u.LastSeenAt, &u.IsOnline, &u.PrivacyLastSeen, &u.PrivacyAvatar, &u.PrivacyAbout, &u.PrivacyReadReceipts, &u.PrivacyPhone, &u.DiscoverableByPhone, &u.AllowGroupAdds, &u.ShareTypingIndicators, &u.SecurityNotifications, &u.CreatedAt, &u.UpdatedAt)
	return &u, err
}
func (r *UserPostgresRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE phone_number=$1 AND deleted_at IS NULL`, userColumns), phone))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user_postgres: find phone: %w", err)
	}
	return u, nil
}
func (r *UserPostgresRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE id=$1 AND deleted_at IS NULL`, userColumns), id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user_postgres: find id: %w", err)
	}
	return u, nil
}
func (r *UserPostgresRepository) FindOrCreateByPhone(ctx context.Context, phone, country string) (*domain.User, bool, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`INSERT INTO users(phone_number,country_code,display_name) VALUES($1,$2,'BharatChat user') ON CONFLICT(phone_number) DO NOTHING RETURNING %s`, userColumns), phone, country)
	u, err := scanUser(row)
	if err == nil {
		return u, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("user_postgres: create: %w", err)
	}
	u, err = r.FindByPhone(ctx, phone)
	if err != nil || u == nil {
		return nil, false, fmt.Errorf("user_postgres: find after conflict: %w", err)
	}
	return u, false, nil
}
func (r *UserPostgresRepository) IsUsernameTaken(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username=$1)`, name).Scan(&exists)
	return exists, err
}
func (r *UserPostgresRepository) UpdateProfile(ctx context.Context, id string, u domain.ProfileUpdate) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`UPDATE users SET display_name=COALESCE($2,display_name),username=COALESCE($3,username),about=COALESCE($4,about),avatar_url=COALESCE($5,avatar_url) WHERE id=$1 AND deleted_at IS NULL RETURNING %s`, userColumns), id, u.DisplayName, u.Username, u.About, u.AvatarURL)
	out, err := scanUser(row)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil, domain.ErrUsernameTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *UserPostgresRepository) UpdatePrivacy(ctx context.Context, id string, update domain.PrivacyUpdate) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE users SET
			privacy_last_seen=COALESCE($2,privacy_last_seen),
			privacy_avatar=COALESCE($3,privacy_avatar),
			privacy_about=COALESCE($4,privacy_about),
			privacy_phone=COALESCE($5,privacy_phone),
			allow_group_adds=COALESCE($6,allow_group_adds),
			privacy_read_receipts=COALESCE($7,privacy_read_receipts),
			discoverable_by_phone=COALESCE($8,discoverable_by_phone),
			security_notifications=COALESCE($9,security_notifications),
			share_typing_indicators=COALESCE($10,share_typing_indicators)
		WHERE id=$1 AND deleted_at IS NULL RETURNING %s`, userColumns),
		id, update.LastSeen, update.Avatar, update.About, update.Phone,
		update.AllowGroupAdds, update.ReadReceipts, update.DiscoverableByPhone,
		update.SecurityNotifications, update.ShareTypingIndicators)
	user, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user_postgres: update privacy: %w", err)
	}
	return user, nil
}

func (r *UserPostgresRepository) CanBeAddedToGroup(ctx context.Context, requesterUserID, targetUserID string) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users target
			WHERE target.id=$2 AND target.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM blocked_users b
				WHERE (b.blocker_id=$1 AND b.blocked_id=$2)
				   OR (b.blocker_id=$2 AND b.blocked_id=$1)
			  )
			  AND (
				target.allow_group_adds='everyone'
				OR (target.allow_group_adds='contacts' AND EXISTS (
					SELECT 1 FROM contacts c
					WHERE c.owner_id=$2 AND c.contact_user_id=$1
				))
			  )
		)`, requesterUserID, targetUserID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("user_postgres: group-add privacy check: %w", err)
	}
	return allowed, nil
}

func (r *UserPostgresRepository) CanStartDirectChat(ctx context.Context, requesterUserID, targetUserID string) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users requester, users target
			WHERE requester.id=$1 AND target.id=$2
			  AND requester.deleted_at IS NULL AND target.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM blocked_users b
				WHERE (b.blocker_id=$1 AND b.blocked_id=$2)
				   OR (b.blocker_id=$2 AND b.blocked_id=$1)
			  )
		)`, requesterUserID, targetUserID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("user_postgres: direct-chat privacy check: %w", err)
	}
	return allowed, nil
}

func (r *UserPostgresRepository) ReadReceiptsEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := r.pool.QueryRow(ctx, `
		SELECT privacy_read_receipts FROM users
		WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("user_postgres: read-receipt preference: %w", err)
	}
	return enabled, nil
}

func (r *UserPostgresRepository) TypingIndicatorsEnabled(ctx context.Context, userID string) (bool, error) {
	var enabled bool
	err := r.pool.QueryRow(ctx, `
		SELECT share_typing_indicators FROM users
		WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("user_postgres: typing-indicator preference: %w", err)
	}
	return enabled, nil
}

func (r *UserPostgresRepository) DeleteAccount(ctx context.Context, userID, sessionID, currentRefreshHash string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("user_postgres: begin account deletion: %w", err)
	}
	defer tx.Rollback(ctx)
	var phone string
	if err := tx.QueryRow(ctx, `SELECT phone_number FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&phone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("user_postgres: lock account for deletion: %w", err)
	}
	// Lock the credential until erasure commits, preventing refresh/revocation
	// from racing the destructive operation after the service's initial check.
	var validSession string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM sessions WHERE id=$1 AND user_id=$2
		AND refresh_token_hash=$3 AND revoked_at IS NULL AND expires_at>now()
		FOR UPDATE`, sessionID, userID, currentRefreshHash).Scan(&validSession); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrDeletionSessionChanged
		}
		return false, fmt.Errorf("user_postgres: lock deletion session: %w", err)
	}
	var affectedChats []string
	rows, err := tx.Query(ctx, `SELECT c.id FROM chats c
		JOIN chat_participants cp ON cp.chat_id=c.id
		WHERE cp.user_id=$1 ORDER BY c.id FOR UPDATE OF c, cp`, userID)
	if err != nil {
		return false, fmt.Errorf("user_postgres: lock account chats: %w", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		affectedChats = append(affectedChats, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	var ownsActiveGroup bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM chat_participants owner
		JOIN chats c ON c.id=owner.chat_id
		WHERE owner.user_id=$1 AND owner.role='owner' AND owner.left_at IS NULL
		AND c.type='group' AND c.deleted_at IS NULL AND EXISTS(
			SELECT 1 FROM chat_participants peer WHERE peer.chat_id=c.id
			AND peer.user_id<>$1 AND peer.left_at IS NULL))`, userID).Scan(&ownsActiveGroup); err != nil {
		return false, fmt.Errorf("user_postgres: check owned groups: %w", err)
	}
	if ownsActiveGroup {
		return false, domain.ErrGroupOwnershipTransferRequired
	}

	// Direct conversations are private two-party aggregates, so erasure removes
	// the aggregate. In groups, only messages authored by the departing account
	// are erased; the user FK cascade removes membership and device metadata.
	if _, err := tx.Exec(ctx, `
		DELETE FROM chats WHERE type='direct' AND id IN (
			SELECT chat_id FROM chat_participants WHERE user_id=$1
		)`, userID); err != nil {
		return false, fmt.Errorf("user_postgres: erase direct chats: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM messages WHERE sender_id=$1`, userID); err != nil {
		return false, fmt.Errorf("user_postgres: erase group messages: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM otp_challenges WHERE phone_number=$1`, phone); err != nil {
		return false, fmt.Errorf("user_postgres: erase otp records: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	if err != nil {
		return false, fmt.Errorf("user_postgres: erase user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM chats c WHERE c.id=ANY($1::uuid[]) AND NOT EXISTS (
			SELECT 1 FROM chat_participants cp WHERE cp.chat_id=c.id AND cp.left_at IS NULL
		)`, affectedChats); err != nil {
		return false, fmt.Errorf("user_postgres: erase orphan chats: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("user_postgres: commit account deletion: %w", err)
	}
	return true, nil
}
