package websocket

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

const (
	writeWait                = 10 * time.Second
	pongWait                 = 60 * time.Second
	pingPeriod               = (pongWait * 9) / 10
	maxMessageSize           = 1 << 20
	authorizationCheckPeriod = 15 * time.Second
	authorizationTimeout     = 3 * time.Second
)

type EnvelopeHandler func(context.Context, string, *Envelope)
type ConnectionAuthorizer func(context.Context) bool
type Client struct {
	conn      *websocket.Conn
	userID    string
	sendCh    chan *Envelope
	hub       *Hub
	log       zerolog.Logger
	onEvent   EnvelopeHandler
	closeOnce sync.Once
	done      chan struct{}
	authorize ConnectionAuthorizer
}

func NewClient(conn *websocket.Conn, userID string, hub *Hub, log zerolog.Logger, onEvent EnvelopeHandler, authorizers ...ConnectionAuthorizer) *Client {
	client := &Client{conn: conn, userID: userID, sendCh: make(chan *Envelope, 64), hub: hub, log: log, onEvent: onEvent, done: make(chan struct{})}
	if len(authorizers) > 0 {
		client.authorize = authorizers[0]
	}
	return client
}
func (c *Client) Send(env *Envelope) {
	select {
	case c.sendCh <- env:
	case <-c.done:
	default:
		c.log.Warn().Str("user_id", c.userID).Msg("client: send buffer full")
	}
}
func (c *Client) close() {
	c.closeOnce.Do(func() { close(c.done); c.hub.Unregister(c.userID, c); _ = c.conn.Close() })
}
func (c *Client) isAuthorized(parent context.Context) bool {
	if c.authorize == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(parent, authorizationTimeout)
	defer cancel()
	if c.authorize(ctx) {
		return true
	}
	_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "session revoked"), time.Now().Add(writeWait))
	c.close()
	return false
}
func (c *Client) Run(ctx context.Context) {
	go c.writePump()
	go func() {
		select {
		case <-ctx.Done():
			c.close()
		case <-c.done:
		}
	}()
	c.readPump(ctx)
}
func (c *Client) readPump(ctx context.Context) {
	defer c.close()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if !c.isAuthorized(ctx) {
			return
		}
		var env Envelope
		if err := jsonUnmarshal(raw, &env); err != nil {
			c.log.Warn().Err(err).Msg("client: invalid inbound envelope")
			continue
		}
		c.onEvent(ctx, c.userID, &env)
	}
}
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	authTicker := time.NewTicker(authorizationCheckPeriod)
	defer ticker.Stop()
	defer authTicker.Stop()
	for {
		select {
		case env := <-c.sendCh:
			if !c.isAuthorized(context.Background()) {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			raw, err := env.Marshal()
			if err != nil {
				continue
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, raw); err != nil {
				c.close()
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.close()
				return
			}
		case <-authTicker.C:
			if !c.isAuthorized(context.Background()) {
				return
			}
		case <-c.done:
			return
		}
	}
}
