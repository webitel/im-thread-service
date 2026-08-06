package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

// #region Queries and Commands
type ResolveThreadQuery struct {
	From   shared.Peer
	To     shared.Peer
	SendAs *uuid.UUID
}

func (q *ResolveThreadQuery) SendAsPtr() *uuid.UUID {
	if q == nil {
		return nil
	}

	if q.SendAs != nil && *q.SendAs != uuid.Nil {
		return q.SendAs
	}

	return nil
}

// #endregion

//go:generate stringer -type=ThreadKind
type ThreadKind int

const (
	ThreadUnspecified ThreadKind = iota
	ThreadDirect
	ThreadGroup
	ThreadChannel
)

type Thread struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	DomainID    int        `json:"domain_id" db:"domain_id"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	Kind        ThreadKind `json:"kind" db:"kind"`
	Subject     string     `json:"subject" db:"subject"`
	Description string     `json:"description" db:"description"`
	Owner       uuid.UUID  `json:"owner"`

	Members []*ThreadDialog `json:"members,omitempty" db:"members"`

	LastMessageID uuid.UUID `json:"last_message_id" db:"last_message_id"`
	LastMessage   *Message  `json:"last_msg,omitempty" db:"last_msg"`

	Variables *ThreadVariables `json:"variables,omitempty" db:"variables"`

	BotControllerID *uuid.UUID `json:"bot_controller_id,omitempty" db:"bot_controller_id"`
	OwnerBotID      *uuid.UUID `json:"owner_bot_id,omitempty" db:"owner_bot_id"`

	// UnreadCount is the number of unread messages in this thread for the
	// requesting participant. Enriched after the thread query; not scanned.
	UnreadCount int64 `json:"unread_count" db:"-"`

	events []event.Base `db:"-"`
}

func (t *Thread) CreatedAtUnix() int64 { return max(t.CreatedAt.UTC().UnixMilli(), 0) }
func (t *Thread) UpdatedAtUnix() int64 { return max(t.UpdatedAt.UTC().UnixMilli(), 0) }

func (t *Thread) AddEvents(e ...event.Base) {
	t.events = append(t.events, e...)
}

func (t *Thread) PullEvents() []event.Base {
	events := t.events
	t.events = nil

	return events
}
