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

type Recipient struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func NewRecipient(id uuid.UUID, name string) *Recipient {
	return &Recipient{
		ID:   id,
		Name: name,
	}
}

type ThreadCreated struct {
	ID        uuid.UUID       `json:"id"`
	DomainID  int32           `json:"domain_id"`
	CreatedAt time.Time       `json:"created_at"`
	Subject   string          `json:"subject"`
	Recipient *Recipient      `json:"recipient"`
	Kind      string          `json:"kind"`
	Members   []*ThreadMember `json:"members"`
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

func (e *ThreadCreated) RecipientID() uuid.UUID { return e.Recipient.ID }

func (e *ThreadCreated) MustBeThreadEvent() {}

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

type ThreadCreatedBuilder struct {
	event *ThreadCreated
}

func NewThreadCreatedBuilder() *ThreadCreatedBuilder {
	return &ThreadCreatedBuilder{
		event: new(ThreadCreated),
	}
}

func (b *ThreadCreatedBuilder) WithID(id uuid.UUID) *ThreadCreatedBuilder {
	b.event.ID = id

	return b
}

func (b *ThreadCreatedBuilder) WithDomainID(domainID int32) *ThreadCreatedBuilder {
	b.event.DomainID = domainID

	return b
}

func (b *ThreadCreatedBuilder) WithCreatedAt(createdAt time.Time) *ThreadCreatedBuilder {
	b.event.CreatedAt = createdAt

	return b
}

func (b *ThreadCreatedBuilder) WithSubject(subject string) *ThreadCreatedBuilder {
	b.event.Subject = subject

	return b
}

func (b *ThreadCreatedBuilder) WithRecipient(recipient *Recipient) *ThreadCreatedBuilder {
	b.event.Recipient = recipient

	return b
}

func (b *ThreadCreatedBuilder) WithKind(kind string) *ThreadCreatedBuilder {
	b.event.Kind = kind

	return b
}

func (b *ThreadCreatedBuilder) WithMembers(members []*ThreadMember) *ThreadCreatedBuilder {
	b.event.Members = members

	return b
}

func (b *ThreadCreatedBuilder) Build() *ThreadCreated {
	return b.event
}
