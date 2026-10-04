package websocket

func (h *Hub) RegisterRecorder(userID string, s sendable) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recorders[userID] = append(h.recorders[userID], s)
}
