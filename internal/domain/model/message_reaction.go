package model

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

// ReactionAction is the outcome of a SetReaction call relative to the reaction
// the reactor previously held on the message.
type ReactionAction string

const (
	ReactionActionUnspecified ReactionAction = ""
	ReactionActionSet         ReactionAction = "set"
	ReactionActionRemoved     ReactionAction = "removed"
	ReactionActionUnchanged   ReactionAction = "unchanged"
)

// Reaction is a single emoji reaction a member holds on a message. At most one
// reaction exists per (MessageID, ReactorID): a new emoji replaces the previous
// one and an empty emoji clears it, giving natural toggle semantics.
type Reaction struct {
	ID              uuid.UUID  `db:"id"`
	MessageID       uuid.UUID  `db:"message_id"`
	ThreadID        uuid.UUID  `db:"thread_id"`
	DomainID        int32      `db:"domain_id"`
	ReactorID       uuid.UUID  `db:"reactor_id"`
	ReactorMemberID *uuid.UUID `db:"reactor_member_id"`
	Emoji           string     `db:"emoji"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`

	// IdempotencyKey / ExternalID carry request context; not persisted here.
	IdempotencyKey string `db:"-"`
	ExternalID     string `db:"-"`

	// Reactor is the peer leaving the reaction; To is the recipient list used
	// to fan the change out to both sides.
	Reactor shared.Peer     `db:"-"`
	To      []*ThreadDialog `db:"-"`

	domainEvents []event.Outboxer
}

// ReactionResult is what SetReaction settled on: the stored emoji, the action
// taken relative to the reactor's previous reaction, and whether anything
// actually changed (a no-op repeat is not re-published).
type ReactionResult struct {
	Action    ReactionAction
	Emoji     string
	ThreadID  uuid.UUID
	ReactedAt time.Time
	Changed   bool
}

// MessageReaction is the read-model projection of reactions on a message,
// aggregated per emoji: how many members hold it, a capped sample of their
// contact ids, and when it last changed. Whether the requesting caller is one
// of the reactors (reacted_by_me) is derived at the mapping layer from the
// caller id, not stored here.
type MessageReaction struct {
	Emoji         string      `json:"emoji" db:"emoji"`
	Count         int32       `json:"count" db:"count"`
	ReactorIDs    []uuid.UUID `json:"reactor_ids" db:"reactor_ids"`
	LastReactedAt int64       `json:"last_reacted_at" db:"last_reacted_at"`
}

// ReactedBy reports whether the given contact is among this emoji's reactors
// (within the sampled set the read model carries).
func (r *MessageReaction) ReactedBy(contactID uuid.UUID) bool {
	return slices.Contains(r.ReactorIDs, contactID)
}

// IsRemoval reports whether an empty emoji clears the reactor's reaction.
func (r *Reaction) IsRemoval() bool {
	return r == nil || r.Emoji == ""
}

// AddEvent stages an outboxer for the service layer.
func (r *Reaction) AddEvent(e event.Outboxer) {
	r.domainEvents = append(r.domainEvents, e)
}

// Events drains the staged events.
func (r *Reaction) Events() []event.Outboxer {
	e := r.domainEvents
	r.domainEvents = nil

	return e
}

// WithReactionEvent stages a MessageReaction event describing the given action.
// It is only meaningful for set/removed; an unchanged result stages nothing.
func (r *Reaction) WithReactionEvent(ctx context.Context, res *ReactionResult) *Reaction {
	if r == nil || res == nil || !res.Changed {
		return r
	}

	action := event.ReactionActionSet
	if res.Action == ReactionActionRemoved {
		action = event.ReactionActionRemoved
	}

	var reactor *event.ThreadMember
	if r.ReactorID != uuid.Nil {
		reactor = &event.ThreadMember{ContactID: r.ReactorID}
		if r.ReactorMemberID != nil && *r.ReactorMemberID != uuid.Nil {
			memberID := *r.ReactorMemberID
			reactor.ID = &memberID
		}
	}

	e := event.MessageReaction{
		MessageID:  r.MessageID,
		ThreadID:   r.ThreadID,
		DomainID:   r.DomainID,
		Reactor:    reactor,
		To:         threadDialogsAsEventMembers(r.To),
		Emoji:      res.Emoji,
		Action:     action,
		OccurredAt: res.ReactedAt,
		SendID:     r.IdempotencyKey,
	}

	WithContextPropogatedMetadata(ctx, &e)

	if via := r.firstViaOrDefault(); via != "" && uuid.Validate(via) == nil {
		e.AddMetadata(XWebitelVia, via)
	}

	r.AddEvent(&e)

	return r
}

func (r *Reaction) firstViaOrDefault() string {
	for _, member := range r.To {
		if via := member.Via; via != nil && *via != "" {
			return *via
		}
	}

	return ""
}
