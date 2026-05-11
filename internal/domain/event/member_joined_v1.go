package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MemberJoinedEvent = "im.thread.member.joined"
)

type MemberJoined struct {
	MessageID  uuid.UUID      `json:"message_id"`
	ThreadID   uuid.UUID      `json:"thread_id"`
	DomainID   int32          `json:"domain_id"`
	ContactID  uuid.UUID      `json:"contact_id"`
	OccurredAt time.Time      `json:"occurred_at"`
	System     *SystemPayload `json:"system,omitempty"`
}

func (e *MemberJoined) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": MemberJoinedEvent,
			"version":    version,
		},
	}, nil
}

func (e *MemberJoined) EventType() string { return MemberJoinedEvent }

func (e *MemberJoined) Version() string { return MessageVersionV1 }

func (e *MemberJoined) RecipientID() uuid.UUID { return e.ThreadID }

func (e *MemberJoined) MustBeThreadEvent() {}

func (e *MemberJoined) ToOutbox() (OutboxEvent, error) {
	return e.serialize(e, e.Version())
}

func (e *MemberJoined) Topic() string {
	return fmt.Sprintf(
		"im_thread.%s.member.%s.joined.%s",
		e.ThreadID,
		e.ContactID,
		e.Version(),
	)
}
