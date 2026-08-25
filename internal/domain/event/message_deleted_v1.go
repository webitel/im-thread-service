package event

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
)

const MessageDeletedEvent = "im.message.deleted"

var _ Outboxer = (*MessageDeleted)(nil)

// MessageDeleted announces that a message was soft-deleted by its author.
// It deliberately carries no body or attachments: the content stays in the
// database for analytics and must never reach clients again.
type MessageDeleted struct {
	MessageID  uuid.UUID       `json:"message_id"`
	ThreadID   uuid.UUID       `json:"thread_id"`
	DomainID   int32           `json:"domain_id"`
	DeletedBy  *Member         `json:"deleted_by,omitempty"`
	To         []*ThreadMember `json:"to,omitempty"`
	Type       string          `json:"type"` // text|document|image|system|interactive|location|contact
	CreatedAt  time.Time       `json:"created_at"`
	OccurredAt time.Time       `json:"occurred_at"`

	ExternalMetadata map[string]string `json:"-"`
}

func (m *MessageDeleted) AddMetadata(key, value string) {
	if m.ExternalMetadata == nil {
		m.ExternalMetadata = make(map[string]string)
	}

	m.ExternalMetadata[key] = value
}

func (*MessageDeleted) EventType() string                { return MessageDeletedEvent }
func (m *MessageDeleted) Version() string                { return MessageVersionV1 }
func (m *MessageDeleted) RecipientID() uuid.UUID         { return m.ThreadID }
func (m *MessageDeleted) ToOutbox() (OutboxEvent, error) { return m.serialize(m.Version()) }

func (m *MessageDeleted) serialize(version string) (OutboxEvent, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return OutboxEvent{}, err
	}

	outboxMetadata := map[string]string{
		"event_type": MessageDeletedEvent,
		"version":    version,
	}

	maps.Copy(outboxMetadata, m.ExternalMetadata)

	return OutboxEvent{
		ID:       uuid.Must(uuid.NewV7()),
		Payload:  payload,
		Metadata: outboxMetadata,
	}, nil
}
