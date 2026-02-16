package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	ThreadVersionV1 string = "v1"
)

const (
	ThreadCreatedEvent string = "im.thread.created"
)

type ThreadCreated struct {
	ID        uuid.UUID `json:"id"`
	DomainID  int32     `json:"domain_id"`
	CreatedAt time.Time `json:"created_at"`
	Recipient uuid.UUID `json:"recipient"`
}

func NewThreadCreated(id, recipient uuid.UUID, domainID int32, createdAt time.Time) *ThreadCreated {
	return &ThreadCreated{
		ID:        id,
		DomainID:  domainID,
		CreatedAt: createdAt,
		Recipient: recipient,
	}
}

func (e *ThreadCreated) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": ThreadCreatedEvent,
			"version":    version,
		},
	}, nil
}

func (e *ThreadCreated) EventType() string { return ThreadCreatedEvent }

func (e *ThreadCreated) Version() string { return ThreadVersionV1 }

func (e *ThreadCreated) RecipientID() uuid.UUID { return e.Recipient }

func (e *ThreadCreated) ToOutbox() (OutboxEvent, error) {
	return e.serialize(e, e.Version())
}

func (e *ThreadCreated) Topic() string {
	return fmt.Sprintf(
		"im_thread.%s.thread.%s.%s",
		e.RecipientID(),
		"created",
		e.Version(),
	)
}

