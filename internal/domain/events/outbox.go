package events

import (
	"encoding/json"

	"github.com/google/uuid"
)

type OutboxEvent struct {
	ID       uuid.UUID
	Payload  []byte
	Metadata map[string]string
}

type Outboxer interface {
	Event
	ToOutbox() (OutboxEvent, error)
}

func (m MessageCreated) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(m)
	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": m.EventType(),
		},
	}, err
}
