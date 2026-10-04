package repository

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
)

type exportSection struct {
	name  string
	query string
}

// WriteAccountExport writes a ZIP containing newline-delimited JSON sections from
// one read-only, repeatable-read snapshot. Authentication secrets, push tokens,
// storage object keys, and private cryptographic material are intentionally absent.
func (r *UserPostgresRepository) WriteAccountExport(ctx context.Context, userID string, destination io.Writer) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return false, fmt.Errorf("user_postgres: begin account export: %w", err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("user_postgres: check account export user: %w", err)
	}
	if !exists {
		return false, nil
	}

	generatedAt := time.Now().UTC()
	archive := zip.NewWriter(destination)
	metadata, err := archive.CreateHeader(exportHeader("metadata.json", generatedAt))
	if err != nil {
		return false, fmt.Errorf("user_postgres: create account export metadata: %w", err)
	}
	if err := json.NewEncoder(metadata).Encode(map[string]any{
		"exportVersion": 1,
		"generatedAt":   generatedAt,
		"format":        "BharatChat account archive (JSON Lines)",
	}); err != nil {
		return false, fmt.Errorf("user_postgres: write account export metadata: %w", err)
	}

	for _, section := range accountExportSections {
		if err := writeExportSection(ctx, tx, archive, generatedAt, section, userID); err != nil {
			return false, err
		}
	}
	if err := archive.Close(); err != nil {
		return false, fmt.Errorf("user_postgres: finalize account export: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("user_postgres: commit account export snapshot: %w", err)
	}
	return true, nil
}

func exportHeader(name string, modified time.Time) *zip.FileHeader {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(modified)
	return header
}

func writeExportSection(ctx context.Context, tx pgx.Tx, archive *zip.Writer, generatedAt time.Time, section exportSection, userID string) error {
	entry, err := archive.CreateHeader(exportHeader(section.name, generatedAt))
	if err != nil {
		return fmt.Errorf("user_postgres: create account export section %s: %w", section.name, err)
	}
	rows, err := tx.Query(ctx, section.query, userID)
	if err != nil {
		return fmt.Errorf("user_postgres: query account export section %s: %w", section.name, err)
	}
	defer rows.Close()

	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return fmt.Errorf("user_postgres: scan account export section %s: %w", section.name, err)
		}
		if _, err := entry.Write(append(raw, '\n')); err != nil {
			return fmt.Errorf("user_postgres: write account export section %s: %w", section.name, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("user_postgres: iterate account export section %s: %w", section.name, err)
	}
	return nil
}

var accountExportSections = []exportSection{
	{name: "profile.jsonl", query: `
		SELECT jsonb_build_object(
			'id', id, 'phoneNumber', phone_number, 'countryCode', country_code,
			'username', username, 'displayName', display_name, 'avatarUrl', avatar_url,
			'about', about, 'isVerified', is_verified, 'lastSeenAt', last_seen_at,
			'privacyLastSeen', privacy_last_seen, 'privacyAvatar', privacy_avatar,
			'privacyAbout', privacy_about, 'privacyReadReceipts', privacy_read_receipts,
			'privacyPhone', privacy_phone, 'discoverableByPhone', discoverable_by_phone,
			'allowGroupAdds', allow_group_adds, 'shareTypingIndicators', share_typing_indicators,
			'securityNotifications', security_notifications, 'createdAt', created_at, 'updatedAt', updated_at
		) FROM users WHERE id=$1 AND deleted_at IS NULL`},
	{name: "devices.jsonl", query: `
		SELECT jsonb_build_object(
			'id', id, 'platform', platform, 'deviceName', device_name,
			'appVersion', app_version, 'installationId', installation_id,
			'lastActiveAt', last_active_at, 'isActive', is_active,
			'createdAt', created_at, 'updatedAt', updated_at
		) FROM devices WHERE user_id=$1 ORDER BY created_at`},
	{name: "sessions.jsonl", query: `
		SELECT jsonb_build_object(
			'id', id, 'deviceId', device_id, 'ipAddress', ip_address,
			'userAgent', user_agent, 'expiresAt', expires_at,
			'revokedAt', revoked_at, 'createdAt', created_at
		) FROM sessions WHERE user_id=$1 ORDER BY created_at`},
	{name: "chat_memberships.jsonl", query: `
		SELECT jsonb_build_object(
			'chatId', cp.chat_id, 'type', c.type, 'role', cp.role,
			'isMuted', cp.is_muted, 'isPinned', cp.is_pinned, 'isArchived', cp.is_archived,
			'lastReadMessageId', cp.last_read_message_id, 'joinedAt', cp.joined_at,
			'leftAt', cp.left_at, 'chatCreatedAt', c.created_at,
			'defaultMessageTtlSeconds', CASE WHEN cp.left_at IS NULL THEN c.default_message_ttl_seconds END,
			'groupName', CASE WHEN cp.left_at IS NULL THEN g.name END,
			'groupDescription', CASE WHEN cp.left_at IS NULL THEN g.description END,
			'groupIconUrl', CASE WHEN cp.left_at IS NULL THEN g.icon_url END
		) FROM chat_participants cp
		JOIN chats c ON c.id=cp.chat_id
		LEFT JOIN groups g ON g.chat_id=c.id
		WHERE cp.user_id=$1 ORDER BY cp.joined_at`},
	{name: "messages.jsonl", query: `
		SELECT jsonb_build_object(
			'id', m.id, 'chatId', m.chat_id, 'senderId', m.sender_id, 'type', m.type,
			'body', CASE WHEN m.is_deleted_for_everyone THEN NULL ELSE m.body END,
			'ciphertextHex', CASE WHEN m.is_deleted_for_everyone THEN NULL ELSE encode(m.ciphertext, 'hex') END,
			'encryptionProtocol', m.encryption_protocol, 'encryptionVersion', m.encryption_version,
			'senderDeviceId', m.sender_device_id, 'replyToMessageId', m.reply_to_message_id,
			'forwardedFromMessageId', m.forwarded_from_message_id, 'forwardCount', m.forward_count,
			'isEdited', m.is_edited, 'editedAt', m.edited_at,
			'isDeletedForEveryone', m.is_deleted_for_everyone, 'deletedAt', m.deleted_at,
			'clientGeneratedId', m.client_generated_id, 'expiresAt', m.expires_at,
			'createdAt', m.created_at, 'updatedAt', m.updated_at
		) FROM chat_participants cp
		JOIN messages m ON m.chat_id=cp.chat_id
		WHERE cp.user_id=$1
		  AND m.created_at >= cp.joined_at
		  AND (cp.left_at IS NULL OR m.created_at <= cp.left_at)
		  AND (m.expires_at IS NULL OR m.expires_at > now())
		ORDER BY m.created_at`},
	{name: "message_status.jsonl", query: `
		SELECT jsonb_build_object(
			'messageId', ms.message_id, 'state', ms.state,
			'deliveredAt', ms.delivered_at, 'readAt', ms.read_at
		) FROM message_status ms
		JOIN messages m ON m.id=ms.message_id
		JOIN chat_participants cp ON cp.chat_id=m.chat_id AND cp.user_id=ms.user_id
		WHERE ms.user_id=$1 AND m.created_at >= cp.joined_at
		  AND (cp.left_at IS NULL OR m.created_at <= cp.left_at)
		  AND (m.expires_at IS NULL OR m.expires_at > now())
		  AND NOT m.is_deleted_for_everyone
		ORDER BY ms.message_id`},
	{name: "reactions.jsonl", query: `
		SELECT jsonb_build_object(
			'messageId', mr.message_id, 'emoji', mr.emoji, 'createdAt', mr.created_at
		) FROM message_reactions mr
		JOIN messages m ON m.id=mr.message_id
		JOIN chat_participants cp ON cp.chat_id=m.chat_id AND cp.user_id=mr.user_id
		WHERE mr.user_id=$1 AND m.created_at >= cp.joined_at
		  AND (cp.left_at IS NULL OR m.created_at <= cp.left_at)
		  AND (m.expires_at IS NULL OR m.expires_at > now())
		  AND NOT m.is_deleted_for_everyone
		ORDER BY mr.created_at`},
	{name: "media.jsonl", query: `
		SELECT jsonb_build_object(
			'id', md.id, 'messageId', md.message_id, 'mimeType', md.mime_type,
			'fileName', md.file_name, 'fileSizeBytes', md.file_size_bytes,
			'width', md.width, 'height', md.height, 'durationMs', md.duration_ms,
			'checksumSha256', md.checksum_sha256, 'isVoiceNote', md.is_voice_note,
			'uploadStatus', md.upload_status, 'createdAt', md.created_at
		) FROM media md WHERE md.uploader_id=$1
		AND (md.message_id IS NULL OR EXISTS (
			SELECT 1 FROM messages m
			JOIN chat_participants cp ON cp.chat_id=m.chat_id AND cp.user_id=$1
			WHERE m.id=md.message_id AND m.created_at >= cp.joined_at
			  AND (cp.left_at IS NULL OR m.created_at <= cp.left_at)
			  AND (m.expires_at IS NULL OR m.expires_at > now())
			  AND NOT m.is_deleted_for_everyone
		)) ORDER BY md.created_at`},
	{name: "contacts.jsonl", query: `
		SELECT jsonb_build_object(
			'id', id, 'contactUserId', contact_user_id, 'savedName', saved_name,
			'phoneNumber', phone_number, 'isFavorite', is_favorite,
			'createdAt', created_at, 'updatedAt', updated_at
		) FROM contacts WHERE owner_id=$1 ORDER BY created_at`},
	{name: "blocked_users.jsonl", query: `
		SELECT jsonb_build_object('blockedUserId', blocked_id, 'createdAt', created_at)
		FROM blocked_users WHERE blocker_id=$1 ORDER BY created_at`},
	{name: "notifications.jsonl", query: `
		SELECT jsonb_build_object(
			'id', id, 'actorId', actor_id, 'type', type, 'chatId', chat_id,
			'messageId', message_id, 'title', title, 'body', body, 'payload', payload,
			'isRead', is_read, 'isPushed', is_pushed, 'createdAt', created_at
		) FROM notifications n WHERE recipient_id=$1
		AND (n.message_id IS NULL OR EXISTS (
			SELECT 1 FROM messages m
			JOIN chat_participants cp ON cp.chat_id=m.chat_id AND cp.user_id=$1
			WHERE m.id=n.message_id AND m.created_at >= cp.joined_at
			  AND (cp.left_at IS NULL OR m.created_at <= cp.left_at)
			  AND (m.expires_at IS NULL OR m.expires_at > now())
			  AND NOT m.is_deleted_for_everyone
		)) ORDER BY created_at`},
	{name: "reports_submitted.jsonl", query: `
		SELECT jsonb_build_object(
			'id', id, 'reportedUserId', reported_user_id,
			'reportedMessageId', reported_message_id, 'reason', reason,
			'details', details, 'status', status, 'createdAt', created_at, 'resolvedAt', resolved_at
		) FROM reports WHERE reporter_id=$1 ORDER BY created_at`},
	{name: "otp_activity.jsonl", query: `
		SELECT jsonb_build_object(
			'id', o.id, 'purpose', o.purpose, 'attemptCount', o.attempt_count,
			'maxAttempts', o.max_attempts, 'expiresAt', o.expires_at,
			'consumedAt', o.consumed_at, 'createdAt', o.created_at
		) FROM otp_challenges o
		JOIN users u ON u.phone_number=o.phone_number
		WHERE u.id=$1 ORDER BY o.created_at`},
	{name: "public_key_directory.jsonl", query: `
		SELECT jsonb_build_object(
			'deviceId', kb.device_id, 'registrationId', kb.registration_id,
			'identityAgreementPublicKeyHex', encode(kb.identity_agreement_public_key, 'hex'),
			'identitySigningPublicKeyHex', encode(kb.identity_signing_public_key, 'hex'),
			'signedPreKeyId', kb.signed_pre_key_id,
			'signedPreKeyPublicKeyHex', encode(kb.signed_pre_key_public_key, 'hex'),
			'signedPreKeySignatureHex', encode(kb.signed_pre_key_signature, 'hex'),
			'keyVersion', kb.key_version, 'createdAt', kb.created_at, 'updatedAt', kb.updated_at
		) FROM device_key_bundles kb WHERE kb.user_id=$1 ORDER BY kb.created_at`},
}
