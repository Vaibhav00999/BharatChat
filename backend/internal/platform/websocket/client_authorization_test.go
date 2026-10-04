package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestRevokedConnectionCannotExchangeQueuedEvents(t *testing.T) {
	for _, direction := range []string{"inbound", "outbound"} {
		t.Run(direction, func(t *testing.T) {
			var allowed atomic.Bool
			allowed.Store(true)
			var handled atomic.Int32
			clients := make(chan *Client, 1)
			hub := NewHub(nil, zerolog.Nop())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&gorilla.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				client := NewClient(conn, "account", hub, zerolog.Nop(), func(context.Context, string, *Envelope) {
					handled.Add(1)
				}, func(ctx context.Context) bool {
					_, bounded := ctx.Deadline()
					return bounded && allowed.Load()
				})
				hub.Register("account", client)
				clients <- client
				client.Run(r.Context())
			}))
			t.Cleanup(server.Close)
			conn, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			var client *Client
			select {
			case client = <-clients:
			case <-time.After(3 * time.Second):
				t.Fatal("connection was not registered")
			}
			allowed.Store(false)
			env, err := NewEnvelope(EventNewMessage, map[string]string{"body": "must not be delivered"})
			require.NoError(t, err)
			if direction == "inbound" {
				require.NoError(t, conn.WriteJSON(env))
			} else {
				client.Send(env)
			}
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
			_, _, err = conn.ReadMessage()
			require.True(t, gorilla.IsCloseError(err, gorilla.ClosePolicyViolation), "expected revocation closure, got %v", err)
			require.Zero(t, handled.Load(), "a revoked connection must not dispatch inbound events")
			select {
			case <-client.done:
			case <-time.After(time.Second):
				t.Fatal("revoked connection did not stop")
			}
		})
	}
}
