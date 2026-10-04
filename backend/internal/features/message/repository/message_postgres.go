// internal/features/message/repository/message_postgres.go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bharatchat/backend/internal/features/message/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MessagePostgresRepository struct {
	pool *pgxpool.Pool
}

func NewMessagePostgresRepository(pool *pgxpool.Pool) *MessagePostgresRepository {
	return &MessagePostgresRepository{pool: pool}
}

const messageColumns = `
	id, chat_id, sender_id, type, body, reply_to_message_id, forwarded_from_message_id,
	forward_count, is_edited, edited_at, is_deleted_for_everyone, deleted_at,
	client_generated_id, created_at, updated_at, ciphertext, encryption_protocol,
	encryption_version, sender_device_id, expires_at`

func scanMessage(row pgx.Row) (*domain.Message, error) {
	var m domain.Message
	var encryptionProtocol *string
	var encryptionVersion *int
	err := row.Scan(
		&m.ID, &m.ChatID, &m.SenderID, &m.Type, &m.Body, &m.ReplyToMessageID, &m.ForwardedFromMessageID,
		&m.ForwardCount, &m.IsEdited, &m.EditedAt, &m.IsDeletedForEveryone, &m.DeletedAt,
		&m.ClientGeneratedID, &m.CreatedAt, &m.UpdatedAt, &m.Ciphertext, &encryptionProtocol,
		&encryptionVersion, &m.SenderDeviceID, &m.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	if encryptionProtocol != nil {
		m.EncryptionProtocol = *encryptionProtocol
	}
	if encryptionVersion != nil {
		m.EncryptionVersion = *encryptionVersion
	}
	return &m, nil
}

// Create inserts the message row and one message_status row per recipient inside a
// single transaction. If msg.ClientGeneratedID is set and a message with the same
// (chat_id, sender_id, client_generated_id) already exists (per the unique index
// from Module 1), this is treated as an idempotent resend: the existing message is
// returned instead of erroring, so a client retry after a dropped ack never creates
// a duplicate message.
func (r *MessagePostgresRepository) Create(ctx context.Context, msg domain.Message, recipientUserIDs []string) (*domain.Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("message_postgres: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var chatType string
	if err := tx.QueryRow(ctx, `SELECT type FROM chats WHERE id=$1 AND deleted_at IS NULL`, msg.ChatID).Scan(&chatType); err != nil {
		return nil, err
	}
	if chatType == "direct" && msg.SenderID != nil {
		var peer string
		if err := tx.QueryRow(ctx, `SELECT user_id FROM chat_participants WHERE chat_id=$1 AND user_id<>$2 AND left_at IS NULL`, msg.ChatID, *msg.SenderID).Scan(&peer); err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('direct-chat:' ||
			LEAST($1::uuid::text, $2::uuid::text) || ':' || GREATEST($1::uuid::text, $2::uuid::text), 0))`, *msg.SenderID, peer)
		if err != nil {
			return nil, err
		}
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM blocked_users WHERE
			(blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1))`, *msg.SenderID, peer).Scan(&blocked); err != nil {
			return nil, err
		}
		if blocked {
			return nil, domain.ErrInteractionBlocked
		}
	}

	// The domain uses zero values for absent encryption metadata, while the
	// database constraint represents absence with SQL NULL. Passing an empty
	// string would make a plaintext development message look like a malformed
	// encrypted message and correctly trip chk_messages_encrypted_content.
	var encryptionProtocol any
	if msg.EncryptionProtocol != "" {
		encryptionProtocol = msg.EncryptionProtocol
	}

	row := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO messages (
			chat_id, sender_id, type, body, reply_to_message_id, client_generated_id,
			ciphertext, encryption_protocol, encryption_version, sender_device_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0), $10)
		ON CONFLICT (chat_id, sender_id, client_generated_id) WHERE client_generated_id IS NOT NULL
		DO NOTHING
		RETURNING %s`, messageColumns),
		msg.ChatID, msg.SenderID, string(msg.Type), msg.Body, msg.ReplyToMessageID, msg.ClientGeneratedID,
		msg.Ciphertext, encryptionProtocol, msg.EncryptionVersion, msg.SenderDeviceID)

	created, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either a genuine conflict (idempotent resend) — fetch and return the existing row.
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("message_postgres: commit tx (idempotent path): %w", commitErr)
		}
		if msg.ClientGeneratedID == nil {
			return nil, fmt.Errorf("message_postgres: insert returned no rows but no client_generated_id to look up")
		}
		existingRow := r.pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT %s FROM messages WHERE chat_id = $1 AND sender_id = $2 AND client_generated_id = $3`, messageColumns),
			msg.ChatID, msg.SenderID, *msg.ClientGeneratedID)
		existing, findErr := scanMessage(existingRow)
		if findErr != nil {
			return nil, fmt.Errorf("message_postgres: find existing after conflict: %w", findErr)
		}
		return existing, nil
	}
	if err != nil {
		return nil, fmt.Errorf("message_postgres: insert message: %w", err)
	}

	if len(recipientUserIDs) > 0 {
		batch := &pgx.Batch{}
		for _, recipientID := range recipientUserIDs {
			batch.Queue(`
				INSERT INTO message_status (message_id, user_id, state)
				VALUES ($1, $2, 'sent')
				ON CONFLICT (message_id, user_id) DO NOTHING`, created.ID, recipientID)
		}
		br := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return nil, fmt.Errorf("message_postgres: insert status row %d: %w", i, err)
			}
		}
		if err := br.Close(); err != nil {
			return nil, fmt.Errorf("message_postgres: close batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("message_postgres: commit tx: %w", err)
	}

	return created, nil
}

func (r *MessagePostgresRepository) ListHistory(ctx context.Context, chatID string, limit int, beforeMessageID *string) ([]domain.Message, error) {
	var rows pgx.Rows
	var err error

	if beforeMessageID != nil {
		rows, err = r.pool.Query(ctx, fmt.Sprintf(`
			SELECT %s FROM messages
			WHERE chat_id = $1 AND is_deleted_for_everyone = FALSE
			  AND (expires_at IS NULL OR expires_at > now())
			  AND created_at < (SELECT created_at FROM messages WHERE id = $2)
			ORDER BY created_at DESC
			LIMIT $3`, messageColumns), chatID, *beforeMessageID, limit)
	} else {
		rows, err = r.pool.Query(ctx, fmt.Sprintf(`
			SELECT %s FROM messages
			WHERE chat_id = $1 AND is_deleted_for_everyone = FALSE
			  AND (expires_at IS NULL OR expires_at > now())
			ORDER BY created_at DESC
			LIMIT $2`, messageColumns), chatID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("message_postgres: list history: %w", err)
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("message_postgres: scan history row: %w", err)
		}
		messages = append(messages, *m)
	}

	return messages, rows.Err()
}

func (r *MessagePostgresRepository) FindByID(ctx context.Context, messageID string) (*domain.Message, error) {
	row := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM messages WHERE id = $1`, messageColumns), messageID)
	m, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("message_postgres: find by id: %w", err)
	}
	return m, nil
}

// stateRank lets UpdateStatus enforce forward-only transitions (sent -> delivered ->
// read) purely in application code — simpler and just as correct as a DB CHECK
// constraint here since this is the only write path for message_status.state.
var stateRank = map[domain.DeliveryState]int{
	domain.StateSent:      0,
	domain.StateDelivered: 1,
	domain.StateRead:      2,
}

func (r *MessagePostgresRepository) UpdateStatus(ctx context.Context, messageID, userID string, newState domain.DeliveryState) (bool, error) {
	if _, ok := stateRank[newState]; !ok || newState == domain.StateSent {
		return false, fmt.Errorf("message_postgres: unsupported target state %q", newState)
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE message_status
		SET state=$3,
		    delivered_at=CASE WHEN $3 IN ('delivered','read') THEN COALESCE(delivered_at,now()) ELSE delivered_at END,
		    read_at=CASE WHEN $3='read' THEN COALESCE(read_at,now()) ELSE read_at END
		WHERE message_id=$1 AND user_id=$2
		  AND CASE state WHEN 'sent' THEN 0 WHEN 'delivered' THEN 1 ELSE 2 END
		      < CASE $3::message_delivery_state WHEN 'sent' THEN 0 WHEN 'delivered' THEN 1 ELSE 2 END`, messageID, userID, string(newState))
	if err != nil {
		return false, fmt.Errorf("message_postgres: update status: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

// MarkAllReadUpTo returns the IDs of every message whose status row was actually
// transitioned to 'read' by this call (so the caller can publish exactly one
// read_ack event per affected message's original sender, without re-notifying
// for messages that were already read).
func (r *MessagePostgresRepository) MarkAllReadUpTo(ctx context.Context, chatID, userID, upToMessageID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE message_status ms
		SET state = 'read', read_at = now()
		FROM messages m
		WHERE ms.message_id = m.id
		  AND m.chat_id = $1
		  AND ms.user_id = $2
		  AND ms.state <> 'read'
		  AND m.created_at <= (SELECT created_at FROM messages WHERE id = $3 AND chat_id = $1)
		RETURNING ms.message_id`, chatID, userID, upToMessageID)
	if err != nil {
		return nil, fmt.Errorf("message_postgres: mark all read up to: %w", err)
	}
	defer rows.Close()

	var messageIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("message_postgres: scan updated message id: %w", err)
		}
		messageIDs = append(messageIDs, id)
	}

	return messageIDs, rows.Err()
}

func (r *MessagePostgresRepository) GetStatusesForMessage(ctx context.Context, messageID string) ([]domain.MessageStatus, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT message_id, user_id, state, delivered_at, read_at
		FROM message_status WHERE message_id = $1`, messageID)
	if err != nil {
		return nil, fmt.Errorf("message_postgres: get statuses: %w", err)
	}
	defer rows.Close()

	var statuses []domain.MessageStatus
	for rows.Next() {
		var s domain.MessageStatus
		if err := rows.Scan(&s.MessageID, &s.UserID, &s.State, &s.DeliveredAt, &s.ReadAt); err != nil {
			return nil, fmt.Errorf("message_postgres: scan status: %w", err)
		}
		statuses = append(statuses, s)
	}

	return statuses, rows.Err()
}
