package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	ThreadUpdatedEvent = "im.thread.thread.updated"
)

// ThreadUpdated reports a change of thread attributes (e.g. the selected CRM contact).
type ThreadUpdated struct {
	Journaled

	ThreadID     uuid.UUID `json:"thread_id"`
	DomainID     int32     `json:"domain_id"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
	CRMContactID string    `json:"crm_contact_id"`
	OccurredAt   time.Time `json:"occurred_at"`
	// Participants are the thread's current members, so delivery fans the event out.
	Participants []uuid.UUID `json:"participants,omitempty"`
}

func (e *ThreadUpdated) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": ThreadUpdatedEvent,
			"version":    version,
		},
	}, nil
}

func (e *ThreadUpdated) EventType() string { return ThreadUpdatedEvent }

func (e *ThreadUpdated) Version() string { return MessageVersionV1 }

func (e *ThreadUpdated) RecipientID() uuid.UUID { return e.ThreadID }

func (e *ThreadUpdated) JournalThreadID() uuid.UUID { return e.ThreadID }

func (e *ThreadUpdated) MustBeThreadEvent() {}

func (e *ThreadUpdated) ToOutbox() (OutboxEvent, error) {
	return e.serialize(e, e.Version())
}

func (e *ThreadUpdated) Topic() string {
	return fmt.Sprintf(
		"im_thread.%s.thread.updated.%s",
		e.ThreadID,
		e.Version(),
	)
}
