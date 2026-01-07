package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event defines the basic identity of a domain event.
type Event interface {
	EventType() string
}

// Outboxer defines the contract for events that must be persisted
// and dispatched via the Transactional Outbox pattern.
type Outboxer interface {
	Event
	// RecipientID returns the target peer ID for precise RabbitMQ routing (e.g., im.message.{id})
	RecipientID() uuid.UUID
	// ToOutbox serializes the event and prepares it for database storage
	ToOutbox() (OutboxEvent, error)
}

// OutboxEvent represents a serialized event ready for the Outbox store.
type OutboxEvent struct {
	ID       uuid.UUID
	Payload  []byte
	Metadata map[string]string
}

// --- Message Events ---

const MessageCreatedEvent = "im.message.created"

// MessageCreated is a flat DTO for message distribution.
// It uses primitive types to maintain zero dependencies on the 'model' package,
// effectively preventing circular imports.
type MessageCreated struct {
	MessageID uuid.UUID `json:"message_id"`
	ThreadID  uuid.UUID `json:"thread_id"`

	// [PEER_DATA] Flattened peer structure for cross-service compatibility
	FromID   uuid.UUID `json:"from_id"`
	FromType int       `json:"from_type"`
	ToID     uuid.UUID `json:"to_id"`
	ToType   int       `json:"to_type"`

	Body string `json:"body"`

	// [MESSAGE_TYPE] Mirrors model.MessageType (1: TEXT, 2: FILE, 3: IMAGE, 4: SYSTEM)
	Type       int16     `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`

	// [ATTACHMENTS] Simplified payloads for external consumers
	Images    []ImagePayload    `json:"images,omitempty"`
	Documents []DocumentPayload `json:"documents,omitempty"`
}

// EventType identifies the event for routing and serialization logic.
func (MessageCreated) EventType() string {
	return MessageCreatedEvent
}

// RecipientID satisfies the Outboxer interface, enabling dynamic Routing Key generation.
func (m MessageCreated) RecipientID() uuid.UUID {
	return m.ToID
}

// ToOutbox transforms the domain event into a persistable Outbox record.
// It uses UUID v7 for time-ordered database indexing.
func (m MessageCreated) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": m.EventType(),
			"version":    "v1",
		},
	}, nil
}

// --- Payloads ---

type ImagePayload struct {
	FileID int64  `json:"file_id"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
}

type DocumentPayload struct {
	FileID int64  `json:"file_id"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}
