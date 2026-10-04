package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeTicketConsumer struct {
	received string
}

func (f *fakeTicketConsumer) Consume(_ context.Context, ticket string) (AuthenticatedSession, bool, error) {
	f.received = ticket
	return AuthenticatedSession{
		UserID:    "b2f3ed0e-e0c4-4f7e-a7e7-803d86295455",
		SessionID: "35ad0554-fe06-4ba5-a95f-ef4c242eec72",
	}, true, nil
}

func TestRequireWebSocketTicketAcceptsSubprotocolCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	consumer := &fakeTicketConsumer{}
	router := gin.New()
	router.GET("/api/v1/ws", RequireWebSocketTicket(consumer), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	ticket := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOP1"
	require.Len(t, ticket, 43)
	request.Header.Set("Sec-WebSocket-Protocol", "bharatchat.v1, ticket."+ticket)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, ticket, consumer.received)
}

func TestRequireWebSocketTicketRejectsURLCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	consumer := &fakeTicketConsumer{}
	router := gin.New()
	router.GET("/api/v1/ws", RequireWebSocketTicket(consumer), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/ws?ticket=abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOP1", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Empty(t, consumer.received)
}
