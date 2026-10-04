package websocket

import "encoding/json"

type EventType string

const (
	EventSendMessage      EventType = "send_message"
	EventMessageDelivered EventType = "message_delivered"
	EventMessageRead      EventType = "message_read"
	EventTypingStart      EventType = "typing_start"
	EventTypingStop       EventType = "typing_stop"
	EventNewMessage       EventType = "new_message"
	EventMessageAck       EventType = "message_ack"
	EventDeliveryAck      EventType = "delivery_ack"
	EventReadAck          EventType = "read_ack"
	EventTypingUpdate     EventType = "typing_update"
	EventPresenceUpdate   EventType = "presence_update"
	EventErrorEvent       EventType = "error"
)

type Envelope struct {
	Type      EventType       `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	RequestID string          `json:"requestId,omitempty"`
}

func NewEnvelope(t EventType, payload interface{}) (*Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Envelope{Type: t, Payload: raw}, nil
}
func (e *Envelope) Marshal() ([]byte, error) { return json.Marshal(e) }
