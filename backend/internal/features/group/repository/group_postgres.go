// internal/features/group/repository/group_postgres.go
package repository

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"

	"github.com/bharatchat/backend/internal/features/group/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewGroupPostgresRepository(pool *pgxpool.Pool) *GroupPostgresRepository {
	return &GroupPostgresRepository{pool: pool}
}

const groupColumns = `
	chat_id, name, description, icon_url, invite_code, invite_code_enabled,
	only_admins_can_post, only_admins_can_edit_info, max_members, created_at, updated_at`

func scanGroup(row pgx.Row) (*domain.GroupInfo, error) {
	var g domain.GroupInfo
	err := row.Scan(
		&g.ChatID, &g.Name, &g.Description, &g.IconURL, &g.InviteCode, &g.InviteCodeEnabled,
		&g.OnlyAdminsCanPost, &g.OnlyAdminsCanEditInfo, &g.MaxMembers, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// generateInviteCode returns a URL-safe, unambiguous 10-character code (base32,
// Crockford-ish charset via standard base32 with padding stripped) — short enough
// to share in a link, long enough (32^10 space) to not be brute-forceable via
// casual guessing within the invite's practical lifetime.
func generateInviteCode() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return encoded[:10], nil
}

func (r *GroupPostgresRepository) Create(ctx context.Context, info domain.GroupInfo) (*domain.GroupInfo, error) {
	code, err := generateInviteCode()
	if err != nil {
		return nil, fmt.Errorf("group_postgres: generate invite code: %w", err)
	}

	row := r.pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO groups (chat_id, name, description, icon_url, invite_code)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING %s`, groupColumns),
		info.ChatID, info.Name, info.Description, info.IconURL, code)

	created, err := scanGroup(row)
	if err != nil {
		return nil, fmt.Errorf("group_postgres: create: %w", err)
	}
	return created, nil
}

func (r *GroupPostgresRepository) FindByChatID(ctx context.Context, chatID string) (*domain.GroupInfo, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM groups WHERE chat_id = $1`, groupColumns), chatID)
	g, err := scanGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("group_postgres: find by chat id: %w", err)
	}
	return g, nil
}

func (r *GroupPostgresRepository) FindByInviteCode(ctx context.Context, inviteCode string) (*domain.GroupInfo, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM groups WHERE invite_code = $1 AND invite_code_enabled = TRUE`, groupColumns), inviteCode)
	g, err := scanGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("group_postgres: find by invite code: %w", err)
	}
	return g, nil
}

func (r *GroupPostgresRepository) UpdateInfo(ctx context.Context, chatID string, name, description, iconURL *string) (*domain.GroupInfo, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`
		UPDATE groups SET
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			icon_url = COALESCE($4, icon_url)
		WHERE chat_id = $1
		RETURNING %s`, groupColumns), chatID, name, description, iconURL)

	g, err := scanGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("group_postgres: update info: %w", err)
	}
	return g, nil
}

func (r *GroupPostgresRepository) SetOnlyAdminsCanPost(ctx context.Context, chatID string, value bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE groups SET only_admins_can_post = $2 WHERE chat_id = $1`, chatID, value)
	if err != nil {
		return fmt.Errorf("group_postgres: set only_admins_can_post: %w", err)
	}
	return nil
}

func (r *GroupPostgresRepository) SetOnlyAdminsCanEditInfo(ctx context.Context, chatID string, value bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE groups SET only_admins_can_edit_info = $2 WHERE chat_id = $1`, chatID, value)
	if err != nil {
		return fmt.Errorf("group_postgres: set only_admins_can_edit_info: %w", err)
	}
	return nil
}

func (r *GroupPostgresRepository) RegenerateInviteCode(ctx context.Context, chatID string) (string, error) {
	newCode, err := generateInviteCode()
	if err != nil {
		return "", fmt.Errorf("group_postgres: generate invite code: %w", err)
	}

	_, err = r.pool.Exec(ctx, `UPDATE groups SET invite_code = $2 WHERE chat_id = $1`, chatID, newCode)
	if err != nil {
		return "", fmt.Errorf("group_postgres: regenerate invite code: %w", err)
	}
	return newCode, nil
}

func (r *GroupPostgresRepository) SetInviteCodeEnabled(ctx context.Context, chatID string, enabled bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE groups SET invite_code_enabled = $2 WHERE chat_id = $1`, chatID, enabled)
	if err != nil {
		return fmt.Errorf("group_postgres: set invite code enabled: %w", err)
	}
	return nil
}

func (r *GroupPostgresRepository) ListMembers(ctx context.Context, chatID, requestingUserID string) ([]domain.MemberView, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			u.id,
			u.display_name,
			CASE
				WHEN u.id = $2 OR u.privacy_avatar = 'everyone' THEN u.avatar_url
				WHEN u.privacy_avatar = 'contacts' AND EXISTS (
					SELECT 1 FROM contacts contact
					WHERE contact.owner_id = u.id AND contact.contact_user_id = $2
				) THEN u.avatar_url
				ELSE NULL
			END AS visible_avatar_url,
			cp.role,
			CASE
				WHEN u.id = $2 OR u.privacy_last_seen = 'everyone' THEN u.is_online
				WHEN u.privacy_last_seen = 'contacts' AND EXISTS (
					SELECT 1 FROM contacts contact
					WHERE contact.owner_id = u.id AND contact.contact_user_id = $2
				) THEN u.is_online
				ELSE FALSE
			END AS visible_online_status,
			cp.joined_at
		FROM chat_participants cp
		JOIN users u ON u.id = cp.user_id
		WHERE cp.chat_id = $1 AND cp.left_at IS NULL
		ORDER BY
			CASE cp.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
			u.display_name ASC`, chatID, requestingUserID)
	if err != nil {
		return nil, fmt.Errorf("group_postgres: list members: %w", err)
	}
	defer rows.Close()

	var members []domain.MemberView
	for rows.Next() {
		var m domain.MemberView
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.AvatarURL, &m.Role, &m.IsOnline, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("group_postgres: scan member: %w", err)
		}
		members = append(members, m)
	}

	return members, rows.Err()
}
