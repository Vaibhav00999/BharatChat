// internal/features/chat/repository/chat_postgres.go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bharatchat/backend/internal/features/chat/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChatPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewChatPostgresRepository(pool *pgxpool.Pool) *ChatPostgresRepository {
	return &ChatPostgresRepository{pool: pool}
}

func scanChat(row pgx.Row) (*domain.Chat, error) {
	var c domain.Chat
	err := row.Scan(&c.ID, &c.Type, &c.CreatedBy, &c.LastMessageID, &c.LastActivityAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

const chatColumns = `id, type, created_by, last_message_id, last_activity_at, created_at, updated_at`

// FindOrCreateDirectChat first looks for an existing direct chat containing exactly
// these two participants (via a self-join on chat_participants), and if none exists,
// creates the chat + both participant rows inside a single transaction. The lookup
// query intentionally checks for a chat with type='direct' having BOTH userA and userB
// as active participants — not just "any chat containing both", since a group chat
// could coincidentally contain both users too.
func (r *ChatPostgresRepository) FindOrCreateDirectChat(ctx context.Context, userA, userB string) (*domain.Chat, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	lockKey := userA + ":" + userB
	if userB < userA {
		lockKey = userB + ":" + userA
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "direct-chat:"+lockKey); err != nil {
		return nil, fmt.Errorf("chat_postgres: acquire direct-chat lock: %w", err)
	}

	existing, err := scanChat(tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM chats c
		WHERE c.type = 'direct' AND c.deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM chat_participants WHERE chat_id=c.id AND user_id=$1 AND left_at IS NULL)
		  AND EXISTS (SELECT 1 FROM chat_participants WHERE chat_id=c.id AND user_id=$2 AND left_at IS NULL)
		LIMIT 1`, chatColumns), userA, userB))
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("chat_postgres: commit existing chat: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("chat_postgres: find direct chat: %w", err)
	}

	var chatID string
	err = tx.QueryRow(ctx, `
		INSERT INTO chats (type, created_by) VALUES ('direct', $1) RETURNING id`, userA).Scan(&chatID)
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: insert chat: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO chat_participants (chat_id, user_id) VALUES ($1, $2), ($1, $3)`,
		chatID, userA, userB)
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: insert participants: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("chat_postgres: commit tx: %w", err)
	}

	return r.FindByID(ctx, chatID)
}

func (r *ChatPostgresRepository) FindByID(ctx context.Context, chatID string) (*domain.Chat, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM chats WHERE id = $1 AND deleted_at IS NULL`, chatColumns), chatID)
	c, err := scanChat(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: find by id: %w", err)
	}
	return c, nil
}

// ListChatSummariesForUser is the single query backing the chat list screen. It:
//  1. finds every chat the user actively participates in,
//  2. for direct chats, joins the OTHER participant's user row for peer display info,
//  3. joins the last message body/type/sender for the preview line,
//  4. computes an unread count by comparing message_status rows in 'sent'/'delivered'
//     state (i.e. not yet 'read') for this user within that chat.
//
// This module deliberately scopes to direct chats only (group chat summaries with
// group name/icon arrive in the Groups module) but the query already tolerates
// type='group' rows gracefully (peer fields simply come back empty) rather than
// crashing, so it does not need to change when groups are added — only extended.
func (r *ChatPostgresRepository) ListChatSummariesForUser(ctx context.Context, userID string) ([]domain.ChatSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			c.id, c.type, c.created_by, c.last_message_id, c.last_activity_at, c.created_at, c.updated_at,
			peer.id AS peer_id,
			peer.display_name AS peer_display_name,
			CASE
				WHEN peer.privacy_avatar = 'everyone' THEN peer.avatar_url
				WHEN peer.privacy_avatar = 'contacts' AND EXISTS (
					SELECT 1 FROM contacts contact
					WHERE contact.owner_id=peer.id AND contact.contact_user_id=$1
				) THEN peer.avatar_url
				ELSE NULL
			END AS peer_avatar_url,
			CASE
				WHEN peer.privacy_last_seen = 'everyone' THEN peer.is_online
				WHEN peer.privacy_last_seen = 'contacts' AND EXISTS (
					SELECT 1 FROM contacts contact
					WHERE contact.owner_id=peer.id AND contact.contact_user_id=$1
				) THEN peer.is_online
				ELSE FALSE
			END AS peer_is_online,
			g.name AS group_name, g.icon_url AS group_icon_url,
			lm.body AS last_message_body, lm.type AS last_message_type, lm.sender_id AS last_message_sender_id,
			sender.display_name AS last_message_sender_name,
			cp.is_muted, cp.is_pinned,
			COALESCE((
				SELECT COUNT(*) FROM message_status ms
				JOIN messages m ON m.id = ms.message_id
				WHERE ms.user_id = $1 AND m.chat_id = c.id AND ms.state <> 'read'
			), 0) AS unread_count
		FROM chats c
		JOIN chat_participants cp ON cp.chat_id = c.id AND cp.user_id = $1 AND cp.left_at IS NULL
		LEFT JOIN chat_participants peer_cp ON peer_cp.chat_id = c.id AND peer_cp.user_id <> $1 AND peer_cp.left_at IS NULL AND c.type = 'direct'
		LEFT JOIN users peer ON peer.id = peer_cp.user_id
		LEFT JOIN groups g ON g.chat_id = c.id AND c.type = 'group'
		LEFT JOIN messages lm ON lm.id = c.last_message_id
		LEFT JOIN users sender ON sender.id = lm.sender_id
		WHERE c.deleted_at IS NULL AND cp.is_archived = FALSE
		ORDER BY c.last_activity_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: list summaries: %w", err)
	}
	defer rows.Close()

	var summaries []domain.ChatSummary
	for rows.Next() {
		var s domain.ChatSummary
		var peerID, peerDisplayName, peerAvatarURL *string
		var peerIsOnline *bool
		var groupName, groupIconURL, lastMessageType, lastMessageSenderName *string

		err := rows.Scan(
			&s.Chat.ID, &s.Chat.Type, &s.Chat.CreatedBy, &s.Chat.LastMessageID, &s.Chat.LastActivityAt,
			&s.Chat.CreatedAt, &s.Chat.UpdatedAt,
			&peerID, &peerDisplayName, &peerAvatarURL, &peerIsOnline,
			&groupName, &groupIconURL,
			&s.LastMessageBody, &lastMessageType, &s.LastMessageSenderID,
			&lastMessageSenderName,
			&s.IsMuted, &s.IsPinned,
			&s.UnreadCount,
		)
		if err != nil {
			return nil, fmt.Errorf("chat_postgres: scan summary row: %w", err)
		}

		if peerID != nil {
			s.PeerUserID = *peerID
		}
		if peerDisplayName != nil {
			s.PeerDisplayName = *peerDisplayName
		}
		s.PeerAvatarURL = peerAvatarURL
		if peerIsOnline != nil {
			s.PeerIsOnline = *peerIsOnline
		}
		s.GroupName = groupName
		s.GroupIconURL = groupIconURL
		if lastMessageType != nil {
			s.LastMessageType = *lastMessageType
		}
		s.LastMessageSenderName = lastMessageSenderName

		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

func (r *ChatPostgresRepository) ListParticipants(ctx context.Context, chatID string) ([]domain.Participant, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, chat_id, user_id, role, is_muted, is_pinned, is_archived, last_read_message_id, joined_at, left_at
		FROM chat_participants
		WHERE chat_id = $1 AND left_at IS NULL`, chatID)
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: list participants: %w", err)
	}
	defer rows.Close()

	var participants []domain.Participant
	for rows.Next() {
		var p domain.Participant
		if err := rows.Scan(&p.ID, &p.ChatID, &p.UserID, &p.Role, &p.IsMuted, &p.IsPinned, &p.IsArchived, &p.LastReadMessageID, &p.JoinedAt, &p.LeftAt); err != nil {
			return nil, fmt.Errorf("chat_postgres: scan participant: %w", err)
		}
		participants = append(participants, p)
	}

	return participants, rows.Err()
}

func (r *ChatPostgresRepository) IsParticipant(ctx context.Context, chatID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM chat_participants WHERE chat_id = $1 AND user_id = $2 AND left_at IS NULL)`,
		chatID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("chat_postgres: is participant: %w", err)
	}
	return exists, nil
}

func (r *ChatPostgresRepository) UpdateLastRead(ctx context.Context, chatID, userID, messageID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE chat_participants SET last_read_message_id = $3
		WHERE chat_id = $1 AND user_id = $2`, chatID, userID, messageID)
	if err != nil {
		return fmt.Errorf("chat_postgres: update last read: %w", err)
	}
	return nil
}

func (r *ChatPostgresRepository) SetMuted(ctx context.Context, chatID, userID string, muted bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE chat_participants SET is_muted = $3 WHERE chat_id = $1 AND user_id = $2`, chatID, userID, muted)
	if err != nil {
		return fmt.Errorf("chat_postgres: set muted: %w", err)
	}
	return nil
}

func (r *ChatPostgresRepository) SetPinned(ctx context.Context, chatID, userID string, pinned bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE chat_participants SET is_pinned = $3 WHERE chat_id = $1 AND user_id = $2`, chatID, userID, pinned)
	if err != nil {
		return fmt.Errorf("chat_postgres: set pinned: %w", err)
	}
	return nil
}

func (r *ChatPostgresRepository) SetArchived(ctx context.Context, chatID, userID string, archived bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE chat_participants SET is_archived = $3 WHERE chat_id = $1 AND user_id = $2`, chatID, userID, archived)
	if err != nil {
		return fmt.Errorf("chat_postgres: set archived: %w", err)
	}
	return nil
}

func (r *ChatPostgresRepository) CreateGroupChat(ctx context.Context, creatorUserID string, memberUserIDs []string) (*domain.Chat, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat_postgres: begin group tx: %w", err)
	}
	defer tx.Rollback(ctx)
	var chatID string
	if err := tx.QueryRow(ctx, `INSERT INTO chats(type,created_by) VALUES('group',$1) RETURNING id`, creatorUserID).Scan(&chatID); err != nil {
		return nil, fmt.Errorf("chat_postgres: create group chat: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO chat_participants(chat_id,user_id,role) VALUES($1,$2,'owner')`, chatID, creatorUserID); err != nil {
		return nil, fmt.Errorf("chat_postgres: add owner: %w", err)
	}
	for _, id := range memberUserIDs {
		if id == creatorUserID {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chat_participants(chat_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT(chat_id,user_id) DO NOTHING`, chatID, id); err != nil {
			return nil, fmt.Errorf("chat_postgres: add member: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("chat_postgres: commit group: %w", err)
	}
	return r.FindByID(ctx, chatID)
}

func (r *ChatPostgresRepository) DeleteGroupChat(ctx context.Context, chatID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM chats WHERE id=$1 AND type='group'`, chatID)
	if err != nil {
		return fmt.Errorf("chat_postgres: delete incomplete group chat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("chat_postgres: group chat not found")
	}
	return nil
}

func (r *ChatPostgresRepository) GetParticipantRole(ctx context.Context, chatID, userID string) (domain.MemberRole, bool, error) {
	var role domain.MemberRole
	err := r.pool.QueryRow(ctx, `SELECT role FROM chat_participants WHERE chat_id=$1 AND user_id=$2 AND left_at IS NULL`, chatID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("chat_postgres: get role: %w", err)
	}
	return role, true, nil
}
func (r *ChatPostgresRepository) UpdateParticipantRole(ctx context.Context, chatID, userID string, role domain.MemberRole) error {
	tag, err := r.pool.Exec(ctx, `UPDATE chat_participants SET role=$3 WHERE chat_id=$1 AND user_id=$2 AND left_at IS NULL`, chatID, userID, string(role))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("chat_postgres: active participant not found")
	}
	return nil
}
func (r *ChatPostgresRepository) AddParticipant(ctx context.Context, chatID, userID string, role domain.MemberRole) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO chat_participants(chat_id,user_id,role) VALUES($1,$2,$3)
		ON CONFLICT(chat_id,user_id) DO UPDATE SET
			left_at=NULL,
			role=CASE
				WHEN chat_participants.left_at IS NULL THEN chat_participants.role
				ELSE EXCLUDED.role
			END,
			joined_at=CASE
				WHEN chat_participants.left_at IS NULL THEN chat_participants.joined_at
				ELSE now()
			END`, chatID, userID, string(role))
	return err
}

// AddParticipantIfBelowLimit serializes membership changes for one chat and
// enforces capacity in the same transaction as the insert/reactivation.
func (r *ChatPostgresRepository) AddParticipantIfBelowLimit(ctx context.Context, chatID, userID string, role domain.MemberRole, limit int) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("chat_postgres: begin add participant: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, chatID); err != nil {
		return false, fmt.Errorf("chat_postgres: lock membership: %w", err)
	}

	var alreadyActive bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chat_participants
			WHERE chat_id=$1 AND user_id=$2 AND left_at IS NULL
		)`, chatID, userID).Scan(&alreadyActive); err != nil {
		return false, fmt.Errorf("chat_postgres: check existing participant: %w", err)
	}
	if alreadyActive {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("chat_postgres: commit existing participant: %w", err)
		}
		return true, nil
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM chat_participants WHERE chat_id=$1 AND left_at IS NULL`, chatID).Scan(&count); err != nil {
		return false, fmt.Errorf("chat_postgres: count participants: %w", err)
	}
	if count >= limit {
		return false, nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO chat_participants(chat_id,user_id,role) VALUES($1,$2,$3)
		ON CONFLICT(chat_id,user_id) DO UPDATE SET left_at=NULL,role=EXCLUDED.role,joined_at=now()`,
		chatID, userID, string(role)); err != nil {
		return false, fmt.Errorf("chat_postgres: add participant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("chat_postgres: commit add participant: %w", err)
	}
	return true, nil
}

// TransferOwnership updates both roles atomically while holding row locks on the
// current and prospective owners.
func (r *ChatPostgresRepository) TransferOwnership(ctx context.Context, chatID, currentOwnerUserID, newOwnerUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("chat_postgres: begin ownership transfer: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentRole, newRole domain.MemberRole
	if err := tx.QueryRow(ctx, `
		SELECT role FROM chat_participants
		WHERE chat_id=$1 AND user_id=$2 AND left_at IS NULL
		FOR UPDATE`, chatID, currentOwnerUserID).Scan(&currentRole); err != nil {
		return fmt.Errorf("chat_postgres: lock current owner: %w", err)
	}
	if currentRole != domain.RoleOwner {
		return fmt.Errorf("chat_postgres: current user is no longer owner")
	}
	if err := tx.QueryRow(ctx, `
		SELECT role FROM chat_participants
		WHERE chat_id=$1 AND user_id=$2 AND left_at IS NULL
		FOR UPDATE`, chatID, newOwnerUserID).Scan(&newRole); err != nil {
		return fmt.Errorf("chat_postgres: lock new owner: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE chat_participants
		SET role=CASE WHEN user_id=$2 THEN 'admin'::member_role ELSE 'owner'::member_role END
		WHERE chat_id=$1 AND user_id IN ($2,$3) AND left_at IS NULL`,
		chatID, currentOwnerUserID, newOwnerUserID); err != nil {
		return fmt.Errorf("chat_postgres: transfer ownership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chat_postgres: commit ownership transfer: %w", err)
	}
	return nil
}
func (r *ChatPostgresRepository) RemoveParticipant(ctx context.Context, chatID, userID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE chat_participants SET left_at=now() WHERE chat_id=$1 AND user_id=$2 AND left_at IS NULL`, chatID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("chat_postgres: active participant not found")
	}
	return nil
}
func (r *ChatPostgresRepository) CountParticipants(ctx context.Context, chatID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_participants WHERE chat_id=$1 AND left_at IS NULL`, chatID).Scan(&n)
	return n, err
}
