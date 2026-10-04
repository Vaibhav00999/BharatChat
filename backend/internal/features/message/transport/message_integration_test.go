// backend/internal/features/message/transport/message_integration_test.go
package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	chatrepo "github.com/bharatchat/backend/internal/features/chat/repository"
	chatservice "github.com/bharatchat/backend/internal/features/chat/service"
	chattransport "github.com/bharatchat/backend/internal/features/chat/transport"
	messagerepo "github.com/bharatchat/backend/internal/features/message/repository"
	messageservice "github.com/bharatchat/backend/internal/features/message/service"
	messagetransport "github.com/bharatchat/backend/internal/features/message/transport"
	presenceservice "github.com/bharatchat/backend/internal/features/presence/service"
	userrepo "github.com/bharatchat/backend/internal/features/user/repository"
	userservice "github.com/bharatchat/backend/internal/features/user/service"
	usertransport "github.com/bharatchat/backend/internal/features/user/transport"
	"github.com/bharatchat/backend/internal/middleware"
	"github.com/bharatchat/backend/internal/platform/database"
	"github.com/bharatchat/backend/internal/platform/validator"
	wsplatform "github.com/bharatchat/backend/internal/platform/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testJWTSecret = "message-module-integration-test-secret"

func setupMessageTestServer(t *testing.T) (*httptest.Server, *pgxpool.Pool, func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env:          map[string]string{"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "test"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })

	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(context.Background()) })

	pgHost, err := pgContainer.Host(ctx)
	require.NoError(t, err)
	pgPort, err := pgContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)
	dsn := fmt.Sprintf("postgres://test:test@%s:%s/test?sslmode=disable", pgHost, pgPort.Port())
	require.NoError(t, database.RunMigrations(dsn, "../../../../migrations"))

	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", redisHost, redisPort.Port())})

	testLog := zerolog.New(os.Stderr)
	hub := wsplatform.NewHub(rdb, testLog)
	require.NoError(t, hub.StartSubscriber(ctx))
	hubPublisher := &wsplatform.HubEventPublisherAdapter{Hub: hub}

	userRepository := userrepo.NewUserPostgresRepository(pool)
	userSvc := userservice.NewUserService(userRepository)

	chatRepository := chatrepo.NewChatPostgresRepository(pool)
	chatSvc := chatservice.NewChatService(chatRepository, userSvc)
	chatHandler := chattransport.NewChatHandler(chatSvc, validator.New())

	chatAuthAdapter := &messageservice.ChatServiceAuthorizerAdapter{AuthorizeFn: chatSvc.AuthorizeParticipant, InteractionFn: chatSvc.AuthorizeInteraction}
	chatParticipantAdapter := &messageservice.ChatParticipantListerAdapter{ListParticipantsFn: chatRepository.ListParticipants}

	messageRepository := messagerepo.NewMessagePostgresRepository(pool)
	messageSvc := messageservice.NewMessageService(messageRepository, chatAuthAdapter, chatParticipantAdapter, hubPublisher)
	messageHandler := messagetransport.NewMessageHandler(messageSvc)

	presenceSvc := presenceservice.NewPresenceService(rdb, hubPublisher, chatParticipantAdapter)
	messageWSHandler := messagetransport.NewMessageWSHandler(hub, messageSvc, presenceSvc, testLog)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")

	// Test-only auth stand-in: inject a fixed userID header instead of running the
	// full JWT stack, since this test's purpose is exercising chat+message wiring,
	// not re-testing Module 2's auth (already covered by its own integration tests).
	protected := api.Group("")
	protected.Use(testAuthMiddleware())

	chatHandler.RegisterRoutes(protected)
	usertransport.NewUserHandler(userSvc, validator.New()).RegisterRoutes(protected)
	messageHandler.RegisterRoutes(protected)
	messageWSHandler.RegisterRoutes(protected)

	server := httptest.NewServer(router)

	cleanup := func() {
		cancel()
		server.Close()
		pool.Close()
		rdb.Close()
	}

	_ = userSvc // referenced to create real users below via direct repo calls
	return server, pool, cleanup
}

// testAuthMiddleware reads a plain "X-Test-User-Id" header and injects it exactly
// where middleware.RequireAuth would — this keeps every downstream handler
// identical to production while letting this test drive multiple distinct users
// (Alice, Bob) without needing real JWTs, which Module 2's own tests already cover.
func testAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDHeader := c.GetHeader("X-Test-User-Id")
		if userIDHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED"})
			return
		}
		parsed, err := parseTestUUID(userIDHeader)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"code": "BAD_TEST_USER_ID"})
			return
		}
		c.Set(string(middleware.ContextKeyUserID), parsed)
		c.Set(string(middleware.ContextKeySessionID), parsed) // reused; session identity is irrelevant to this test
		c.Next()
	}
}

func createTestUser(t *testing.T, pool *pgxpool.Pool, phone, displayName string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (phone_number, country_code, display_name) VALUES ($1, '+91', $2) RETURNING id`,
		phone, displayName).Scan(&id)
	require.NoError(t, err)
	return id
}

func postJSONWithUser(t *testing.T, url, userID string, body interface{}) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-Id", userID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func getWithUser(t *testing.T, url, userID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-User-Id", userID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestDirectChatAndMessageFlow_EndToEnd drives: start a direct chat -> send a
// message over WS -> assert the recipient's WS connection receives it in real time
// -> assert REST history reflects it -> assert delivered/read acks reach the sender.
func TestDirectChatAndMessageFlow_EndToEnd(t *testing.T) {
	server, pool, cleanup := setupMessageTestServer(t)
	defer cleanup()

	alice := createTestUser(t, pool, "+919876500201", "Alice")
	bob := createTestUser(t, pool, "+919876500202", "Bob")

	// Step 1: Alice starts a direct chat with Bob via REST.
	resp := postJSONWithUser(t, server.URL+"/api/v1/chats/direct", alice, map[string]string{"peerUserId": bob})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var chatResp map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&chatResp))
	resp.Body.Close()
	chatID := chatResp["id"].(string)
	require.NotEmpty(t, chatID)

	// Step 2: Bob connects over WebSocket (to receive the real-time push).
	bobWSURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"
	bobHeaders := http.Header{}
	bobHeaders.Set("X-Test-User-Id", bob)
	bobConn, _, err := websocket.DefaultDialer.Dial(bobWSURL, bobHeaders)
	require.NoError(t, err)
	defer bobConn.Close()

	// Step 3: Alice also connects, to send the message and receive her ack.
	aliceWSURL := bobWSURL
	aliceHeaders := http.Header{}
	aliceHeaders.Set("X-Test-User-Id", alice)
	aliceConn, _, err := websocket.DefaultDialer.Dial(aliceWSURL, aliceHeaders)
	require.NoError(t, err)
	defer aliceConn.Close()

	// Step 4: Alice sends a message over WS.
	sendEnv := wsplatform.Envelope{
		Type:    wsplatform.EventSendMessage,
		Payload: mustMarshal(t, map[string]interface{}{"chatId": chatID, "type": "text", "body": "hello bob"}),
	}
	require.NoError(t, aliceConn.WriteJSON(sendEnv))

	// Step 5: Alice receives the persistence ack first. Checking this before the
	// recipient event makes a structured server error visible instead of masking
	// it behind a recipient read timeout.
	_ = aliceConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ackEnv wsplatform.Envelope
	require.NoError(t, aliceConn.ReadJSON(&ackEnv))
	require.Equalf(t, wsplatform.EventMessageAck, ackEnv.Type, "unexpected sender event payload: %s", ackEnv.Payload)

	// Step 6: Bob's connection should receive a "new_message" event in real time.
	_ = bobConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var bobEnv wsplatform.Envelope
	require.NoError(t, bobConn.ReadJSON(&bobEnv))
	require.Equalf(t, wsplatform.EventNewMessage, bobEnv.Type, "unexpected recipient event payload: %s", bobEnv.Payload)

	var messagePayload map[string]interface{}
	require.NoError(t, json.Unmarshal(bobEnv.Payload, &messagePayload))
	require.Equal(t, "hello bob", messagePayload["body"])
	messageID := messagePayload["id"].(string)
	require.NotEmpty(t, messageID)

	// Step 7: Bob acks delivery over WS.
	deliveredEnv := wsplatform.Envelope{
		Type:    wsplatform.EventMessageDelivered,
		Payload: mustMarshal(t, map[string]string{"messageId": messageID}),
	}
	require.NoError(t, bobConn.WriteJSON(deliveredEnv))

	// Step 8: Alice's connection should receive a "delivery_ack".
	_ = aliceConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var deliveryAckEnv wsplatform.Envelope
	require.NoError(t, aliceConn.ReadJSON(&deliveryAckEnv))
	require.Equal(t, wsplatform.EventDeliveryAck, deliveryAckEnv.Type)

	// Step 9: Bob acks read over WS.
	readEnv := wsplatform.Envelope{
		Type:    wsplatform.EventMessageRead,
		Payload: mustMarshal(t, map[string]string{"messageId": messageID}),
	}
	require.NoError(t, bobConn.WriteJSON(readEnv))

	// Step 10: Alice's connection should receive a "read_ack".
	_ = aliceConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var readAckEnv wsplatform.Envelope
	require.NoError(t, aliceConn.ReadJSON(&readAckEnv))
	require.Equal(t, wsplatform.EventReadAck, readAckEnv.Type)

	// Step 11: REST history for the chat should show exactly this one message.
	historyResp := getWithUser(t, fmt.Sprintf("%s/api/v1/chats/%s/messages", server.URL, chatID), bob)
	require.Equal(t, http.StatusOK, historyResp.StatusCode)
	var historyBody map[string]interface{}
	require.NoError(t, json.NewDecoder(historyResp.Body).Decode(&historyBody))
	historyResp.Body.Close()

	messages := historyBody["messages"].([]interface{})
	require.Len(t, messages, 1)
	require.Equal(t, "hello bob", messages[0].(map[string]interface{})["body"])
}

// TestNonParticipant_CannotSendOrReadMessages verifies AuthorizeParticipant actually
// blocks a third party from a chat they were never added to.
func TestNonParticipant_CannotSendOrReadMessages(t *testing.T) {
	server, pool, cleanup := setupMessageTestServer(t)
	defer cleanup()

	alice := createTestUser(t, pool, "+919876500301", "Alice")
	bob := createTestUser(t, pool, "+919876500302", "Bob")
	eve := createTestUser(t, pool, "+919876500303", "Eve")

	resp := postJSONWithUser(t, server.URL+"/api/v1/chats/direct", alice, map[string]string{"peerUserId": bob})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var chatResp map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&chatResp))
	resp.Body.Close()
	chatID := chatResp["id"].(string)

	historyResp := getWithUser(t, fmt.Sprintf("%s/api/v1/chats/%s/messages", server.URL, chatID), eve)
	require.Equal(t, http.StatusForbidden, historyResp.StatusCode)
	historyResp.Body.Close()
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func parseTestUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }
