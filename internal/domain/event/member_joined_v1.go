package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MemberJoinedEvent = "im.message.member.joined"
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
		"im_message.%s.message.member.%s.%s",
		e.RecipientID(),
		"joined",
		e.Version(),
	)
}

type MemberJoinedBuilder struct {
	event *MemberJoined
}

func NewMemberJoinedBuilder() *MemberJoinedBuilder {
	return &MemberJoinedBuilder{
		event: new(MemberJoined),
	}
}

func (b *MemberJoinedBuilder) WithMessageID(messageID uuid.UUID) *MemberJoinedBuilder {
	b.event.MessageID = messageID
	return b
}

func (b *MemberJoinedBuilder) WithThreadID(threadID uuid.UUID) *MemberJoinedBuilder {
	b.event.ThreadID = threadID
	return b
}

func (b *MemberJoinedBuilder) WithDomainID(domainID int32) *MemberJoinedBuilder {
	b.event.DomainID = domainID
	return b
}

func (b *MemberJoinedBuilder) WithContactID(contactID uuid.UUID) *MemberJoinedBuilder {
	b.event.ContactID = contactID
	return b
}

func (b *MemberJoinedBuilder) WithOccurredAt(occurredAt time.Time) *MemberJoinedBuilder {
	b.event.OccurredAt = occurredAt
	return b
}

func (b *MemberJoinedBuilder) WithSystem(system *SystemPayload) *MemberJoinedBuilder {
	b.event.System = system
	return b
}

func (b *MemberJoinedBuilder) Build() *MemberJoined {
	return b.event
}
