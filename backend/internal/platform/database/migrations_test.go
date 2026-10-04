// backend/internal/platform/database/migrations_test.go
package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// spinUpPostgres starts an ephemeral Postgres container for the duration of the test.
func spinUpPostgres(t *testing.T) (dsn string, terminate func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "bharatchat_test",
			"POSTGRES_PASSWORD": "bharatchat_test",
			"POSTGRES_DB":       "bharatchat_test",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn = fmt.Sprintf("postgres://bharatchat_test:bharatchat_test@%s:%s/bharatchat_test?sslmode=disable", host, port.Port())

	terminate = func() {
		_ = container.Terminate(ctx)
	}
	return dsn, terminate
}

func runMigrations(t *testing.T, dsn string) *migrate.Migrate {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	require.NoError(t, err)

	m, err := migrate.NewWithDatabaseInstance("file://../../../migrations", "pgx", driver)
	require.NoError(t, err)

	require.NoError(t, m.Up())
	return m
}

func TestMigrations_UpAndDown_Succeed(t *testing.T) {
	dsn, terminate := spinUpPostgres(t)
	defer terminate()

	m := runMigrations(t, dsn)

	// Down-migrate everything to prove every .down.sql file is valid and reversible.
	err := m.Down()
	require.NoError(t, err)
}

func TestSchema_CoreTablesExist(t *testing.T) {
	dsn, terminate := spinUpPostgres(t)
	defer terminate()
	runMigrations(t, dsn)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	expectedTables := []string{
		"users", "devices", "sessions", "otp_challenges",
		"chats", "chat_participants", "groups",
		"contacts", "blocked_users",
		"messages", "media", "media_purposes",
		"message_reactions", "message_status",
		"notifications", "reports",
		"device_key_bundles", "one_time_pre_keys",
	}

	for _, table := range expectedTables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists)
		require.NoError(t, err, "querying existence of table %s", table)
		require.True(t, exists, "expected table %s to exist after migrations", table)
	}
}

func TestSchema_ForeignKeys_MessagesAndChats(t *testing.T) {
	dsn, terminate := spinUpPostgres(t)
	defer terminate()
	runMigrations(t, dsn)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	// chats.last_message_id -> messages.id must exist as a deferred FK (added in migration 000007)
	var constraintExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE constraint_name = 'fk_chats_last_message'
			AND table_name = 'chats'
		)`).Scan(&constraintExists)
	require.NoError(t, err)
	require.True(t, constraintExists, "expected fk_chats_last_message constraint to exist")
}

func TestTrigger_TouchChatOnNewMessage(t *testing.T) {
	dsn, terminate := spinUpPostgres(t)
	defer terminate()
	runMigrations(t, dsn)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	var userID, chatID string
	require.NoError(t, db.QueryRow(`
		INSERT INTO users (phone_number, country_code, display_name)
		VALUES ('+919876500001', '+91', 'Test User') RETURNING id`).Scan(&userID))

	require.NoError(t, db.QueryRow(`
		INSERT INTO chats (type, created_by) VALUES ('direct', $1) RETURNING id`, userID).Scan(&chatID))

	var beforeActivity time.Time
	require.NoError(t, db.QueryRow(`SELECT last_activity_at FROM chats WHERE id = $1`, chatID).Scan(&beforeActivity))

	time.Sleep(10 * time.Millisecond)

	var messageID string
	require.NoError(t, db.QueryRow(`
		INSERT INTO messages (chat_id, sender_id, type, body)
		VALUES ($1, $2, 'text', 'hello world') RETURNING id`, chatID, userID).Scan(&messageID))

	var lastMessageID string
	var afterActivity time.Time
	require.NoError(t, db.QueryRow(`
		SELECT last_message_id, last_activity_at FROM chats WHERE id = $1`, chatID).
		Scan(&lastMessageID, &afterActivity))

	require.Equal(t, messageID, lastMessageID, "chats.last_message_id should be updated by trigger")
	require.True(t, afterActivity.After(beforeActivity), "chats.last_activity_at should advance on new message")
}

func TestConstraint_OneReactionPerUserPerMessage(t *testing.T) {
	dsn, terminate := spinUpPostgres(t)
	defer terminate()
	runMigrations(t, dsn)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	var userID, chatID, messageID string
	require.NoError(t, db.QueryRow(`
		INSERT INTO users (phone_number, country_code, display_name)
		VALUES ('+919876500002', '+91', 'Reactor') RETURNING id`).Scan(&userID))
	require.NoError(t, db.QueryRow(`
		INSERT INTO chats (type, created_by) VALUES ('direct', $1) RETURNING id`, userID).Scan(&chatID))
	require.NoError(t, db.QueryRow(`
		INSERT INTO messages (chat_id, sender_id, type, body)
		VALUES ($1, $2, 'text', 'react to me') RETURNING id`, chatID, userID).Scan(&messageID))

	_, err = db.Exec(`INSERT INTO message_reactions (message_id, user_id, emoji) VALUES ($1, $2, '👍')`, messageID, userID)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO message_reactions (message_id, user_id, emoji) VALUES ($1, $2, '❤️')`, messageID, userID)
	require.Error(t, err, "expected unique constraint violation for second reaction from same user on same message")
}
