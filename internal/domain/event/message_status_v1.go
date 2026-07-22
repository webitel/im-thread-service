package event

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
)

const (
	MessageStatusChangedEvent = "im.message.status"
)

var _ Outboxer = (*MessageStatusChanged)(nil)

// MessageStatusChanged is published when the per-recipient delivery status
// of one or more messages actually changes (delivered/read/failed).
// Read receipts are bulk ("read up to"), so a single event may cover
// multiple messages of the same recipient in the same thread.
type MessageStatusChanged struct {
	ThreadID uuid.UUID `json:"thread_id"`
	DomainID int32     `json:"domain_id"`
	// MemberID is the recipient contact id (thread_dialog.member_id)
	// whose statuses changed.
	MemberID   uuid.UUID   `json:"member_id"`
	MessageIDs []uuid.UUID `json:"message_ids"`
	// Status is the new delivery state: delivered|read|failed.
	Status string `json:"status"`
	// Via is the confirmation source: ws|push|provider|bot.
	Via        string         `json:"via,omitempty"`
	Error      map[string]any `json:"error,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
	// Participants are the contact ids of all current thread members, so
	// im-delivery can fan the event out without resolving the thread.
	Participants []uuid.UUID `json:"participants,omitempty"`

	ExternalMetadata map[string]string `json:"-"`
}

func (m *MessageStatusChanged) AddMetadata(key, value string) {
	if m.ExternalMetadata == nil {
		m.ExternalMetadata = make(map[string]string)
	}

	m.ExternalMetadata[key] = value
}

func (*MessageStatusChanged) EventType() string        { return MessageStatusChangedEvent }
func (m *MessageStatusChanged) Version() string        { return MessageVersionV1 }
func (m *MessageStatusChanged) RecipientID() uuid.UUID { return m.ThreadID }

func (m *MessageStatusChanged) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return OutboxEvent{}, err
	}

	outboxMetadata := map[string]string{
		"event_type": MessageStatusChangedEvent,
		"version":    m.Version(),
	}

	maps.Copy(outboxMetadata, m.ExternalMetadata)

	return OutboxEvent{
		ID:       uuid.Must(uuid.NewV7()),
		Payload:  payload,
		Metadata: outboxMetadata,
	}, nil
}
