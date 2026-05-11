package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MemberLeftEvent = "im.message.member.left"
)

type MemberLeft struct {
	MessageID  uuid.UUID      `json:"message_id"`
	ThreadID   uuid.UUID      `json:"thread_id"`
	DomainID   int32          `json:"domain_id"`
	ContactID  uuid.UUID      `json:"contact_id"`
	OccurredAt time.Time      `json:"occurred_at"`
	System     *SystemPayload `json:"system,omitempty"`
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

func (e *MemberLeft) MustBeThreadEvent() {}

func (e *MemberLeft) ToOutbox() (OutboxEvent, error) {
	return e.serialize(e, e.Version())
}

func (e *MemberLeft) Topic() string {
	return fmt.Sprintf(
		"im_message.%s.message.member.%s.%s",
		e.RecipientID(),
		"left",
		e.Version(),
	)
}

type MemberLeftBuilder struct {
	event *MemberLeft
}

func NewMemberLeftBuilder() *MemberLeftBuilder {
	return &MemberLeftBuilder{
		event: new(MemberLeft),
	}
}

func (b *MemberLeftBuilder) WithMessageID(messageID uuid.UUID) *MemberLeftBuilder {
	b.event.MessageID = messageID
	return b
}

func (b *MemberLeftBuilder) WithThreadID(threadID uuid.UUID) *MemberLeftBuilder {
	b.event.ThreadID = threadID
	return b
}

func (b *MemberLeftBuilder) WithDomainID(domainID int32) *MemberLeftBuilder {
	b.event.DomainID = domainID
	return b
}

func (b *MemberLeftBuilder) WithContactID(contactID uuid.UUID) *MemberLeftBuilder {
	b.event.ContactID = contactID
	return b
}

func (b *MemberLeftBuilder) WithOccurredAt(occurredAt time.Time) *MemberLeftBuilder {
	b.event.OccurredAt = occurredAt
	return b
}

func (b *MemberLeftBuilder) WithSystem(system *SystemPayload) *MemberLeftBuilder {
	b.event.System = system
	return b
}

func (b *MemberLeftBuilder) Build() *MemberLeft {
	return b.event
}
