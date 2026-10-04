package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

type contextKey string

const (
	ContextKeyUserID    contextKey = "auth.user_id"
	ContextKeySessionID contextKey = "auth.session_id"
)

type AccessClaims struct {
	UserID, SessionID uuid.UUID
	JTI               string
}
type TokenParser interface {
	ParseAccessToken(string) (*AccessClaims, error)
}
type TokenBlacklistChecker interface {
	IsRevoked(context.Context, string) (bool, error)
}
type SessionStatusChecker interface {
	IsSessionActive(context.Context, string, string) (bool, error)
}

type AuthenticatedSession struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
}

type WebSocketTicketConsumer interface {
	Consume(context.Context, string) (AuthenticatedSession, bool, error)
}

func RequireAuth(parser TokenParser, blacklist TokenBlacklistChecker, sessionCheckers ...SessionStatusChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ""
		header := c.GetHeader("Authorization")
		if strings.HasPrefix(header, "Bearer ") {
			raw = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		}
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "missing or malformed authorization header"})
			return
		}
		claims, err := parser.ParseAccessToken(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "invalid or expired access token"})
			return
		}
		revoked, err := blacklist.IsRevoked(c.Request.Context(), claims.JTI)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "AUTH_UNAVAILABLE", "message": "authentication service temporarily unavailable"})
			return
		}
		if revoked {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "access token has been revoked"})
			return
		}
		if len(sessionCheckers) > 0 {
			active, err := sessionCheckers[0].IsSessionActive(c.Request.Context(), claims.SessionID.String(), claims.UserID.String())
			if err != nil {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "AUTH_UNAVAILABLE", "message": "authentication service temporarily unavailable"})
				return
			}
			if !active {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "session has been revoked"})
				return
			}
		}
		c.Set(string(ContextKeyUserID), claims.UserID)
		c.Set(string(ContextKeySessionID), claims.SessionID)
		c.Next()
	}
}

// RequireWebSocketTicket authenticates the browser-compatible WebSocket
// handshake with a short-lived, single-use credential. Long-lived JWTs are
// never placed in URLs where reverse proxies and browser history may retain them.
func RequireWebSocketTicket(consumer WebSocketTicketConsumer) gin.HandlerFunc {
	return func(c *gin.Context) {
		ticket := websocketTicketFromSubprotocol(c.GetHeader("Sec-WebSocket-Protocol"))
		if ticket == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "missing WebSocket ticket"})
			return
		}
		session, ok, err := consumer.Consume(c.Request.Context(), ticket)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "AUTH_UNAVAILABLE", "message": "authentication service temporarily unavailable"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "invalid or expired WebSocket ticket"})
			return
		}
		userID, userErr := uuid.Parse(session.UserID)
		sessionID, sessionErr := uuid.Parse(session.SessionID)
		if userErr != nil || sessionErr != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": "invalid WebSocket ticket"})
			return
		}
		c.Set(string(ContextKeyUserID), userID)
		c.Set(string(ContextKeySessionID), sessionID)
		c.Next()
	}
}

func websocketTicketFromSubprotocol(header string) string {
	for _, protocol := range strings.Split(header, ",") {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, "ticket.") {
			ticket := strings.TrimPrefix(protocol, "ticket.")
			if len(ticket) == 43 {
				return ticket
			}
		}
	}
	return ""
}
func UserIDFromContext(c *gin.Context) uuid.UUID {
	v, ok := c.Get(string(ContextKeyUserID))
	if !ok {
		panic("middleware: missing authenticated user")
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		panic("middleware: invalid authenticated user")
	}
	return id
}
func SessionIDFromContext(c *gin.Context) uuid.UUID {
	v, ok := c.Get(string(ContextKeySessionID))
	if !ok {
		panic("middleware: missing authenticated session")
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		panic("middleware: invalid authenticated session")
	}
	return id
}
