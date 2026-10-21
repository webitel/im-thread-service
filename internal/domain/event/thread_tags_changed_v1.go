package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	ThreadTagsChangedEvent = "im.thread.tags.changed"
)

var _ JournalEvent = (*ThreadTagsChanged)(nil)

// ThreadTagsChanged reports that a contact's own tags on a thread changed. Tags are private,
// so the event reaches only that contact.
type ThreadTagsChanged struct {
	Journaled

	ThreadID     uuid.UUID   `json:"thread_id"`
	DomainID     int32       `json:"domain_id"`
	ContactID    uuid.UUID   `json:"contact_id"`
	Tags         []string    `json:"tags"`
	OccurredAt   time.Time   `json:"occurred_at"`
	Participants []uuid.UUID `json:"participants,omitempty"`
}

func NewThreadTagsChanged(threadID, contactID uuid.UUID, domainID int32, tags []string) *ThreadTagsChanged {
	if tags == nil {
		tags = make([]string, 0)
	}

	return &ThreadTagsChanged{
		ThreadID:     threadID,
		DomainID:     domainID,
		ContactID:    contactID,
		Tags:         tags,
		OccurredAt:   time.Now().UTC(),
		Participants: []uuid.UUID{contactID},
	}
}

func (e *ThreadTagsChanged) EventType() string { return ThreadTagsChangedEvent }

func (e *ThreadTagsChanged) Version() string { return MessageVersionV1 }

func (e *ThreadTagsChanged) RecipientID() uuid.UUID { return e.ThreadID }

func (e *ThreadTagsChanged) JournalThreadID() uuid.UUID { return e.ThreadID }

func (e *ThreadTagsChanged) JournalAudience() []uuid.UUID { return []uuid.UUID{e.ContactID} }

func (e *ThreadTagsChanged) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": ThreadTagsChangedEvent,
			"version":    e.Version(),
		},
	}, nil
}

func (e *ThreadTagsChanged) Topic() string {
	return fmt.Sprintf("im_thread.%s.thread.tags.%s", e.ThreadID, e.Version())
}
