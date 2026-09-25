package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MemberLeftEvent = "im.thread.member.left"
)

type MemberLeft struct {
	MessageID  uuid.UUID      `json:"message_id"`
	ThreadID   uuid.UUID      `json:"thread_id"`
	DomainID   int32          `json:"domain_id"`
	ContactID  uuid.UUID      `json:"contact_id"`
	OccurredAt time.Time      `json:"occurred_at"`
	System     *SystemPayload `json:"system,omitempty"`
	// Participants are the thread's current members, so delivery fans the event out.
	Participants []uuid.UUID `json:"participants,omitempty"`
}

func (e *MemberLeft) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": MemberLeftEvent,
			"version":    version,
		},
	}, nil
}

func (e *MemberLeft) EventType() string { return MemberLeftEvent }

func (e *MemberLeft) Version() string { return MessageVersionV1 }

func (e *MemberLeft) RecipientID() uuid.UUID { return e.ThreadID }

func (e *MemberLeft) JournalThreadID() uuid.UUID { return e.ThreadID }

// JournalSubject is marked too: a contact who just left is no longer an active member.
func (e *MemberLeft) JournalSubject() uuid.UUID { return e.ContactID }

func (e *MemberLeft) MustBeThreadEvent() {}

func (e *MemberLeft) ToOutbox() (OutboxEvent, error) {
	return e.serialize(e, e.Version())
}

func (e *MemberLeft) Topic() string {
	return fmt.Sprintf(
		"im_thread.%s.member.%s.left.%s",
		e.ThreadID,
		e.ContactID,
		e.Version(),
	)
}
