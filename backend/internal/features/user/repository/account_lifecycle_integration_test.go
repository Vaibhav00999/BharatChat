package repository_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bharatchat/backend/internal/features/user/domain"
	"github.com/bharatchat/backend/internal/features/user/repository"
	"github.com/bharatchat/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func lifecycleDatabase(t *testing.T) (*pgxpool.Pool, *repository.UserPostgresRepository) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:16-alpine", ExposedPorts: []string{"5432/tcp"},
			Env:        map[string]string{"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "test"},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute),
		}, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	dsn := fmt.Sprintf("postgres://test:test@%s:%s/test?sslmode=disable", host, port.Port())
	require.NoError(t, database.RunMigrations(dsn, "../../../../migrations"))
	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, repository.NewUserPostgresRepository(pool)
}

func lifecycleID(t *testing.T, pool *pgxpool.Pool, sql string, arguments ...any) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(), sql, arguments...).Scan(&id))
	return id
}

func lifecycleExec(t *testing.T, pool *pgxpool.Pool, sql string, arguments ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, arguments...)
	require.NoError(t, err)
}

func TestDeletionTransactionPreservesGroupsAndUnrelatedChats(t *testing.T) {
	pool, repo := lifecycleDatabase(t)
	ctx := context.Background()
	phone := "+919876501001"
	owner := lifecycleID(t, pool, `INSERT INTO users(phone_number,country_code,display_name) VALUES($1,'+91','Owner') RETURNING id`, phone)
	peer := lifecycleID(t, pool, `INSERT INTO users(phone_number,country_code,display_name) VALUES('+919876501002','+91','Peer') RETURNING id`)
	device := lifecycleID(t, pool, `INSERT INTO devices(user_id,platform) VALUES($1,'web') RETURNING id`, owner)
	hash := strings.Repeat("a", 64)
	session := lifecycleID(t, pool, `INSERT INTO sessions(user_id,device_id,refresh_token_hash,expires_at) VALUES($1,$2,$3,now()+interval '1 hour') RETURNING id`, owner, device, hash)
	chat := lifecycleID(t, pool, `INSERT INTO chats(type,created_by) VALUES('group',$1) RETURNING id`, owner)
	lifecycleExec(t, pool, `INSERT INTO groups(chat_id,name) VALUES($1,'Shared group')`, chat)
	lifecycleExec(t, pool, `INSERT INTO chat_participants(chat_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, chat, owner, peer)
	orphan := lifecycleID(t, pool, `INSERT INTO chats(type) VALUES('group') RETURNING id`)
	lifecycleExec(t, pool, `INSERT INTO otp_challenges(phone_number,otp_hash,expires_at) VALUES($1,'private-otp-hash',now()+interval '5 minutes')`, phone)

	deleted, err := repo.DeleteAccount(ctx, owner, session, strings.Repeat("b", 64))
	require.ErrorIs(t, err, domain.ErrDeletionSessionChanged)
	require.False(t, deleted)
	deleted, err = repo.DeleteAccount(ctx, owner, session, hash)
	require.ErrorIs(t, err, domain.ErrGroupOwnershipTransferRequired)
	require.False(t, deleted)

	lifecycleExec(t, pool, `UPDATE chat_participants SET role='member' WHERE chat_id=$1 AND user_id=$2`, chat, owner)
	lifecycleExec(t, pool, `UPDATE chat_participants SET role='owner' WHERE chat_id=$1 AND user_id=$2`, chat, peer)
	deleted, err = repo.DeleteAccount(ctx, owner, session, hash)
	require.NoError(t, err)
	require.True(t, deleted)
	var users, sessions, otps, orphanCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE id=$1),
		(SELECT count(*) FROM sessions WHERE user_id=$1),
		(SELECT count(*) FROM otp_challenges WHERE phone_number=$2),
		(SELECT count(*) FROM chats WHERE id=$3)`, owner, phone, orphan).Scan(&users, &sessions, &otps, &orphanCount))
	require.Zero(t, users)
	require.Zero(t, sessions)
	require.Zero(t, otps)
	require.Equal(t, 1, orphanCount)
	var role string
	require.NoError(t, pool.QueryRow(ctx, `SELECT role FROM chat_participants WHERE chat_id=$1 AND user_id=$2`, chat, peer).Scan(&role))
	require.Equal(t, "owner", role)
}

func TestExportSnapshotFiltersMessageVisibilityAndSecrets(t *testing.T) {
	pool, repo := lifecycleDatabase(t)
	owner := lifecycleID(t, pool, `INSERT INTO users(phone_number,country_code,display_name) VALUES('+919876502001','+91','Export owner') RETURNING id`)
	peer := lifecycleID(t, pool, `INSERT INTO users(phone_number,country_code,display_name) VALUES('+919876502002','+91','Other profile') RETURNING id`)
	chat := lifecycleID(t, pool, `INSERT INTO chats(type,created_by) VALUES('group',$1) RETURNING id`, peer)
	lifecycleExec(t, pool, `INSERT INTO groups(chat_id,name) VALUES($1,'Export group')`, chat)
	lifecycleExec(t, pool, `INSERT INTO chat_participants(chat_id,user_id,joined_at) VALUES($1,$2,now()-interval '1 hour')`, chat, owner)
	lifecycleExec(t, pool, `INSERT INTO messages(chat_id,sender_id,body,created_at) VALUES($1,$2,'before-joining-secret',now()-interval '2 hours')`, chat, peer)
	lifecycleExec(t, pool, `INSERT INTO messages(chat_id,sender_id,body) VALUES($1,$2,'visible-message')`, chat, peer)
	lifecycleExec(t, pool, `INSERT INTO messages(chat_id,sender_id,body,expires_at) VALUES($1,$2,'expired-message-secret',now()-interval '1 second')`, chat, peer)
	lifecycleExec(t, pool, `INSERT INTO messages(chat_id,sender_id,body,is_deleted_for_everyone) VALUES($1,$2,'tombstone-body-secret',true)`, chat, peer)
	messageRows, err := pool.Query(context.Background(), `SELECT id FROM messages WHERE chat_id=$1`, chat)
	require.NoError(t, err)
	var messageIDs []string
	for messageRows.Next() {
		var messageID string
		require.NoError(t, messageRows.Scan(&messageID))
		messageIDs = append(messageIDs, messageID)
	}
	require.NoError(t, messageRows.Err())
	messageRows.Close()
	for _, messageID := range messageIDs {
		lifecycleExec(t, pool, `INSERT INTO message_status(message_id,user_id) VALUES($1,$2)`, messageID, owner)
		lifecycleExec(t, pool, `INSERT INTO message_reactions(message_id,user_id,emoji) VALUES($1,$2,'ok')`, messageID, owner)
		lifecycleExec(t, pool, `INSERT INTO media(message_id,uploader_id,storage_key,storage_bucket,mime_type,file_name,file_size_bytes)
			SELECT id,$2,'private-storage-key','private-bucket','text/plain',body||'.txt',1 FROM messages WHERE id=$1`, messageID, owner)
		lifecycleExec(t, pool, `INSERT INTO notifications(recipient_id,type,message_id,title,body)
			SELECT $2,'message',id,'Message',body FROM messages WHERE id=$1`, messageID, owner)
	}
	formerGroup := lifecycleID(t, pool, `INSERT INTO chats(type,created_by) VALUES('group',$1) RETURNING id`, peer)
	lifecycleExec(t, pool, `INSERT INTO groups(chat_id,name,description) VALUES($1,'changed-after-left-secret','new-description-secret')`, formerGroup)
	lifecycleExec(t, pool, `INSERT INTO chat_participants(chat_id,user_id,joined_at,left_at) VALUES($1,$2,now()-interval '2 hours',now()-interval '1 hour')`, formerGroup, owner)
	unrelated := lifecycleID(t, pool, `INSERT INTO chats(type) VALUES('direct') RETURNING id`)
	lifecycleExec(t, pool, `INSERT INTO messages(chat_id,sender_id,body) VALUES($1,$2,'unrelated-message-secret')`, unrelated, peer)
	device := lifecycleID(t, pool, `INSERT INTO devices(user_id,platform,push_token) VALUES($1,'web','push-token-secret') RETURNING id`, owner)
	lifecycleExec(t, pool, `INSERT INTO sessions(user_id,device_id,refresh_token_hash,expires_at) VALUES($1,$2,'refresh-hash-secret',now()+interval '1 hour')`, owner, device)

	var buffer bytes.Buffer
	found, err := repo.WriteAccountExport(context.Background(), owner, &buffer)
	require.NoError(t, err)
	require.True(t, found)
	archive, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	require.NoError(t, err)
	sections := make(map[string]string)
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		contents, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		sections[file.Name] = string(contents)
	}
	require.Contains(t, sections["profile.jsonl"], "+919876502001")
	require.NotContains(t, sections["profile.jsonl"], "+919876502002")
	require.Contains(t, sections["messages.jsonl"], "visible-message")
	for _, name := range []string{"message_status.jsonl", "reactions.jsonl", "media.jsonl", "notifications.jsonl"} {
		lines := strings.Split(strings.TrimSpace(sections[name]), "\n")
		require.Len(t, lines, 1, "only visible, non-deleted message records belong in %s", name)
	}
	for _, secret := range []string{"before-joining-secret", "expired-message-secret", "tombstone-body-secret", "unrelated-message-secret", "push-token-secret", "refresh-hash-secret", "changed-after-left-secret", "new-description-secret", "private-storage-key", "private-bucket"} {
		for name, content := range sections {
			require.NotContains(t, content, secret, "section %s", name)
		}
	}
}
