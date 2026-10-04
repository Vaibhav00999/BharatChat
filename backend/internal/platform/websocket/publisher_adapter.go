package websocket

import (
	"context"
	"encoding/json"
)

type HubEventPublisherAdapter struct{ Hub *Hub }

func (a *HubEventPublisherAdapter) PublishToUser(ctx context.Context, userID, eventType string, payload interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return a.Hub.PublishToUser(ctx, userID, &Envelope{Type: EventType(eventType), Payload: raw})
}
