package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

const (
	MessageVersionV1 = "v1"
	MessageVersionV2 = "v2"
)

const (
	MessageCreatedEvent = "im.message.created"
)

type Outboxer interface {
	EventType() string
	Version() string
	RecipientID() uuid.UUID
	ToOutbox() (OutboxEvent, error)
}

type OutboxerTemplate interface {
	Outboxer
	SetRecipientID(uuid.UUID) Outboxer
}

type OutboxEvent struct {
	ID       uuid.UUID
	Payload  []byte
	Metadata map[string]string
}

var (
	_ Outboxer = (*MessageCreated)(nil)
	_ Outboxer = (*MessageCreatedV2)(nil)
)

type MessageCreated struct {
	MessageID  uuid.UUID         `json:"message_id"`
	ThreadID   uuid.UUID         `json:"thread_id"`
	DomainID   int32             `json:"domain_id"`
	From       *shared.Peer      `json:"from"`
	To         uuid.UUIDs        `json:"to"`
	SendID     string            `json:"send_id"`
	Body       string            `json:"body"`
	Type       int16             `json:"type"`
	OccurredAt time.Time         `json:"occurred_at"`
	Images     []ImagePayload    `json:"images,omitempty"`
	Documents  []DocumentPayload `json:"documents,omitempty"`
}

func (MessageCreated) EventType() string        { return MessageCreatedEvent }
func (m MessageCreated) Version() string        { return MessageVersionV1 }
func (m MessageCreated) RecipientID() uuid.UUID { return m.ThreadID }
func (m MessageCreated) ToOutbox() (OutboxEvent, error) {
	return m.serialize(m, m.Version())
}

type MessageCreatedV2 struct {
	MessageCreated
}

func (MessageCreatedV2) EventType() string        { return MessageCreatedEvent }
func (m MessageCreatedV2) Version() string        { return MessageVersionV2 }
func (m MessageCreatedV2) RecipientID() uuid.UUID { return m.ThreadID }
func (m MessageCreatedV2) ToOutbox() (OutboxEvent, error) {
	return m.serialize(m, m.Version())
}

func (m MessageCreated) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": MessageCreatedEvent,
			"version":    version,
		},
	}, nil
}

type ImagePayload struct {
	FileID int64  `json:"file_id"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

type DocumentPayload struct {
	FileID int64  `json:"file_id"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}
