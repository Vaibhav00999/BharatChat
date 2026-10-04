package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	chatrepo "github.com/bharatchat/backend/internal/features/chat/repository"
	msgdomain "github.com/bharatchat/backend/internal/features/message/domain"
	messagerepo "github.com/bharatchat/backend/internal/features/message/repository"
	wsplatform "github.com/bharatchat/backend/internal/platform/websocket"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestSafetyDiscoveryBlockingAndReporting(t *testing.T) {
	server, pool, cleanup := setupMessageTestServer(t)
	defer cleanup()
	ctx := context.Background()
	alice := createTestUser(t, pool, "+919876506001", "Alice")
	bob := createTestUser(t, pool, "+919876506002", "Bob")
	_, err := pool.Exec(ctx, `UPDATE users SET username='bobby' WHERE id=$1`, bob)
	require.NoError(t, err)
	api := server.URL + "/api/v1"
	resp := postJSONWithUser(t, api+"/users/resolve", alice, map[string]string{"username": " @BOBBY "})
	require.Equal(t, 200, resp.StatusCode)
	var person map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&person))
	resp.Body.Close()
	require.Equal(t, bob, person["id"])
	require.Len(t, person, 3, "discovery must not expose phone, presence, or private profile fields")
	chat, err := chatrepo.NewChatPostgresRepository(pool).FindOrCreateDirectChat(ctx, alice, bob)
	require.NoError(t, err)
	changeBlock := func(method, viewer, target string, status int) {
		req, err := http.NewRequest(method, api+"/users/me/blocked/"+target, nil)
		require.NoError(t, err)
		req.Header.Set("X-Test-User-Id", viewer)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, status, res.StatusCode)
	}
	changeBlock("PUT", alice, alice, 400)
	changeBlock("PUT", alice, bob, 204)
	changeBlock("PUT", alice, bob, 204)
	resp = getWithUser(t, api+"/users/me/blocked", alice)
	var blocked struct {
		Users []map[string]interface{} `json:"users"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&blocked))
	resp.Body.Close()
	require.Len(t, blocked.Users, 1)
	resp = postJSONWithUser(t, api+"/users/resolve", alice, map[string]string{"username": "bobby"})
	require.Equal(t, 404, resp.StatusCode)
	resp.Body.Close()
	resp = postJSONWithUser(t, api+"/chats/direct", bob, map[string]string{"peerUserId": alice})
	require.Equal(t, 403, resp.StatusCode)
	resp.Body.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/ws", http.Header{"X-Test-User-Id": {bob}})
	require.NoError(t, err)
	defer conn.Close()
	for _, event := range []wsplatform.EventType{wsplatform.EventSendMessage, wsplatform.EventTypingStart} {
		require.NoError(t, conn.WriteJSON(wsplatform.Envelope{Type: event, Payload: mustMarshal(t, map[string]string{"chatId": chat.ID, "type": "text", "body": "blocked"})}))
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		var reply wsplatform.Envelope
		require.NoError(t, conn.ReadJSON(&reply))
		require.Equal(t, wsplatform.EventErrorEvent, reply.Type)
		require.Contains(t, string(reply.Payload), "FORBIDDEN")
	}
	repository := messagerepo.NewMessagePostgresRepository(pool)
	body := "blocked"
	_, err = repository.Create(ctx, msgdomain.Message{ChatID: chat.ID, SenderID: &bob, Type: msgdomain.MessageTypeText, Body: &body}, []string{alice})
	require.ErrorIs(t, err, msgdomain.ErrInteractionBlocked, "persistence must enforce blocks even if service precheck raced")
	resp = getWithUser(t, api+"/chats/"+chat.ID+"/messages", alice)
	require.Equal(t, 200, resp.StatusCode, "blocking must preserve access to history")
	resp.Body.Close()
	resp = postJSONWithUser(t, api+"/users/me/reports", alice, map[string]string{"userId": bob, "reason": "spam", "details": "Repeated unwanted contact"})
	require.Equal(t, 201, resp.StatusCode)
	resp.Body.Close()
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reports WHERE reporter_id=$1 AND reported_user_id=$2 AND reason='spam'`, alice, bob).Scan(&count))
	require.Equal(t, 1, count)
	changeBlock("DELETE", bob, alice, 204)
	_, err = repository.Create(ctx, msgdomain.Message{ChatID: chat.ID, SenderID: &bob, Type: msgdomain.MessageTypeText, Body: &body}, []string{alice})
	require.ErrorIs(t, err, msgdomain.ErrInteractionBlocked, "another user cannot remove your block")
	changeBlock("DELETE", alice, bob, 204)
	_, err = repository.Create(ctx, msgdomain.Message{ChatID: chat.ID, SenderID: &bob, Type: msgdomain.MessageTypeText, Body: &body}, []string{alice})
	require.NoError(t, err)
}
