package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

// #region Queries and Commands
type ResolveThreadQuery struct {
	From shared.Peer
	To   shared.Peer
}

// #endregion

//go:generate stringer -type=ThreadKind
type ThreadKind int

const (
	ThreadUnspecified ThreadKind = iota
	ThreadDirect                 // user to user
	ThreadGroup                  //group with many users, bots etc.
	ThreadChannel                //channel, right now not implemented!
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
