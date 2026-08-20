package event

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
)

const MessageEditedEvent = "im.message.edited"

var _ Outboxer = (*MessageEdited)(nil)

type MessageEdited struct {
	MessageID  uuid.UUID       `json:"message_id"`
	ThreadID   uuid.UUID       `json:"thread_id"`
	DomainID   int32           `json:"domain_id"`
	EditedBy   *ThreadMember   `json:"edited_by,omitempty"`
	To         []*ThreadMember `json:"to,omitempty"`
	Body       string          `json:"body"`
	Type       string          `json:"type"` // text|document|image|system|interactive|location|contact
	Revision   int32           `json:"version"`
	CreatedAt  time.Time       `json:"created_at"`
	OccurredAt time.Time       `json:"occurred_at"`
	Metadata   map[string]any  `json:"metadata,omitempty"`

	ExternalMetadata map[string]string `json:"-"`
}

func (m *MessageEdited) AddMetadata(key, value string) {
	if m.ExternalMetadata == nil {
		m.ExternalMetadata = make(map[string]string)
	}

	m.ExternalMetadata[key] = value
}

func (*MessageEdited) EventType() string                { return MessageEditedEvent }
func (m *MessageEdited) Version() string                { return MessageVersionV1 }
func (m *MessageEdited) RecipientID() uuid.UUID         { return m.ThreadID }
func (m *MessageEdited) ToOutbox() (OutboxEvent, error) { return m.serialize(m.Version()) }

func (m *MessageEdited) serialize(version string) (OutboxEvent, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return OutboxEvent{}, err
	}

	outboxMetadata := map[string]string{
		"event_type": MessageEditedEvent,
		"version":    version,
	}

	maps.Copy(outboxMetadata, m.ExternalMetadata)

	return OutboxEvent{
		ID:       uuid.Must(uuid.NewV7()),
		Payload:  payload,
		Metadata: outboxMetadata,
	}, nil
}
