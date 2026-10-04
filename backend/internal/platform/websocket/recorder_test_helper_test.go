package websocket_test

import (
	"sync"

	wsplatform "github.com/bharatchat/backend/internal/platform/websocket"
)

type recordingClient struct {
	mu       sync.Mutex
	received []*wsplatform.Envelope
}

func (r *recordingClient) Send(env *wsplatform.Envelope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, env)
}

func (r *recordingClient) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.received)
}

func (r *recordingClient) first() *wsplatform.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.received) == 0 {
		return nil
	}
	return r.received[0]
}
