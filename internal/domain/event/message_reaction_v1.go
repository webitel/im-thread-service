package event

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
)

const MessageReactionEvent = "im.message.reaction"

// Reaction action values carried by MessageReaction.Action.
const (
	ReactionActionSet     = "set"
	ReactionActionRemoved = "removed"
)

// ReactionAggregate is one emoji's authoritative state on the message after the
// change: how many members hold it and a capped sample of their contact ids.
// reacted_by_me is caller-relative and derived downstream from ReactorIDs.
type ReactionAggregate struct {
	Emoji         string      `json:"emoji"`
	Count         int32       `json:"count"`
	ReactorIDs    []uuid.UUID `json:"reactor_ids"`
	LastReactedAt int64       `json:"last_reacted_at"`
}

var _ Outboxer = (*MessageReaction)(nil)

// MessageReaction announces that a member set or cleared an emoji reaction on a
// message. It fans out to every thread participant so both sides converge on
// the same reaction set. A removal carries an empty Emoji and Action=removed.
type MessageReaction struct {
	MessageID  uuid.UUID       `json:"message_id"`
	ThreadID   uuid.UUID       `json:"thread_id"`
	DomainID   int32           `json:"domain_id"`
	Reactor    *ThreadMember   `json:"reactor,omitempty"`
	To         []*ThreadMember `json:"to,omitempty"`
	Emoji      string          `json:"emoji"`
	Action     string          `json:"action"` // set|removed
	OccurredAt time.Time       `json:"occurred_at"`

	// SendID echoes the client-supplied correlation token back to the reactor's
	// devices so an optimistic UI update can be reconciled. It is opaque and
	// plays no part in dedup or ordering.
	SendID string `json:"send_id,omitempty"`

	// Reactions is the full per-emoji aggregate on the message AFTER this change,
	// so a client can replace its reaction state authoritatively rather than
	// applying just the (Emoji, Action) delta.
	Reactions []ReactionAggregate `json:"reactions,omitempty"`

	ExternalMetadata map[string]string `json:"-"`
}

func (m *MessageReaction) AddMetadata(key, value string) {
	if m.ExternalMetadata == nil {
		m.ExternalMetadata = make(map[string]string)
	}

	m.ExternalMetadata[key] = value
}

func (*MessageReaction) EventType() string                { return MessageReactionEvent }
func (m *MessageReaction) Version() string                { return MessageVersionV1 }
func (m *MessageReaction) RecipientID() uuid.UUID         { return m.ThreadID }
func (m *MessageReaction) ToOutbox() (OutboxEvent, error) { return m.serialize(m.Version()) }

func (m *MessageReaction) serialize(version string) (OutboxEvent, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return OutboxEvent{}, err
	}

	outboxMetadata := map[string]string{
		"event_type": MessageReactionEvent,
		"version":    version,
	}

	maps.Copy(outboxMetadata, m.ExternalMetadata)

	return OutboxEvent{
		ID:       uuid.Must(uuid.NewV7()),
		Payload:  payload,
		Metadata: outboxMetadata,
	}, nil
}
