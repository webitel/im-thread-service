// Package events defines the domain-level contracts and data structures for asynchronous
// communication within the IM ecosystem.
//
// It implements the Transactional Outbox pattern, providing a unified Outboxer interface
// that ensures domain events are correctly serialized and staged for persistent delivery.
//
// Key Features:
//   - Versioned Schemas: Supports evolution of event structures (v1, v2) without breaking changes.
//   - Decoupling: Uses primitive-based DTOs to prevent circular dependencies between 'model' and 'events'.
//   - Reliability: Leverages UUID v7 for time-ordered event tracking and efficient database indexing.
//   - Routing: Provides built-in recipient-based routing logic for message broker exchanges.
package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

// [VERSIONS] Explicit API versioning constants for schema evolution
const (
	MessageVersionV1 = "v1"
	MessageVersionV2 = "v2"
)

// [EVENT_TYPES] Unique identifiers for domain occurrences
const (
	MessageCreatedEvent = "im.message.created"
)

// [OUTBOXER_CONTRACT] The primary interface for Transactional Outbox participants.
// Bridges domain logic with persistent asynchronous delivery.
type Outboxer interface {
	EventType() string              // Logical name (e.g., "im.message.created")
	Version() string                // Schema version (e.g., "v1")
	RecipientID() uuid.UUID         // Target ID for message broker routing
	ToOutbox() (OutboxEvent, error) // Self-serialization for DB storage
}

// [OUTBOX_RECORD] Persistable envelope for the outbox store
type OutboxEvent struct {
	ID       uuid.UUID
	Payload  []byte
	Metadata map[string]string
}

// [INTERFACE_GUARDS] Compile-time verification of interface compliance
var (
	_ Outboxer = (*MessageCreated)(nil)
	_ Outboxer = (*MessageCreatedV2)(nil)
)

// -----------------------------------------------------------------------------
// MESSAGE CREATED V1
// -----------------------------------------------------------------------------

// MessageCreated is the baseline schema for message distribution events.
type MessageCreated struct {
	MessageID  uuid.UUID         `json:"message_id"`
	ThreadID   uuid.UUID         `json:"thread_id"`
	DomainID   int32             `json:"domain_id"`
	From       *shared.Peer      `json:"from"`
	To         *shared.Peer      `json:"to"`
	SendID     string            `json:"send_id"`
	Body       string            `json:"body"`
	Type       int16             `json:"type"` // 1:TEXT, 2:FILE, 3:IMAGE, 4:SYSTEM
	OccurredAt time.Time         `json:"occurred_at"`
	Images     []ImagePayload    `json:"images,omitempty"`
	Documents  []DocumentPayload `json:"documents,omitempty"`
}

// [OUTBOXER_IMPLEMENTATION]
func (MessageCreated) EventType() string        { return MessageCreatedEvent }
func (m MessageCreated) Version() string        { return MessageVersionV1 }
func (m MessageCreated) RecipientID() uuid.UUID { return m.To.ID }
func (m MessageCreated) ToOutbox() (OutboxEvent, error) {
	return m.serialize(m, m.Version())
}

// -----------------------------------------------------------------------------
// MESSAGE CREATED V2 (STUB)
// -----------------------------------------------------------------------------

// MessageCreatedV2 embeds V1 to support future schema extensions.
type MessageCreatedV2 struct {
	MessageCreated // [EMBEDDING] Inherit all V1 properties
	// [STUB_NEW_FIELDS] Add v2-specific fields here
}

// [OUTBOXER_IMPLEMENTATION]
func (MessageCreatedV2) EventType() string        { return MessageCreatedEvent }
func (m MessageCreatedV2) Version() string        { return MessageVersionV2 }
func (m MessageCreatedV2) RecipientID() uuid.UUID { return m.To.ID }
func (m MessageCreatedV2) ToOutbox() (OutboxEvent, error) {
	return m.serialize(m, m.Version())
}

// -----------------------------------------------------------------------------
// COMMON INFRASTRUCTURE
// -----------------------------------------------------------------------------

// serialize centralizes the logic for creating OutboxRecords.
func (m MessageCreated) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()), // [ORDERING] Chronological ID
		Payload: payload,
		Metadata: map[string]string{
			"event_type": MessageCreatedEvent,
			"version":    version,
		},
	}, nil
}

// [PAYLOADS] Flattened data structures for cross-service compatibility
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
