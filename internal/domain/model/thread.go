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

type ThreadCountersQuery struct {
	DomainID   int32
	MemberID   uuid.UUID
	Kinds      []ThreadKind
	SnoozedTag string
}

// #endregion

type ThreadCounters struct {
	All     FolderCounters
	Active  FolderCounters
	Snoozed FolderCounters
}

type FolderCounters struct {
	Total       int32
	UnreadChats int32
}

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

	// CRMContactID is the Webitel CRM contact an operator selected for the thread.
	CRMContactID string `json:"crm_contact_id,omitempty" db:"crm_contact_id"`
	// Source is the provider gate of the thread. Nil when no member has a gate; Name/Type are empty
	// while the gate has no reference row yet.
	Source *ThreadSource `json:"source,omitempty" db:"source"`
	// Queue is the call_center queue of the requesting agent. Nil when not set or the queue has no reference row.
	Queue *ThreadQueue `json:"queue,omitempty" db:"queue"`

	// UnreadCount is the number of unread messages in this thread for the
	// requesting participant. Enriched after the thread query; not scanned.
	UnreadCount int64 `json:"unread_count" db:"-"`

	// Tags are the requesting contact's own tags; enriched after the query, not scanned.
	Tags []*ThreadTag `json:"tags,omitempty" db:"-"`

	// TagLookupID is the id enrichment keys tags off. Usually equals ID, but
	// for SearchLeft results ID holds the thread_dialog membership-period
	// row id (not the thread id), so this carries the real thread id.
	TagLookupID uuid.UUID `json:"-" db:"-"`

	// ReadStates is the per-member delivery/read horizon snapshot. Enriched after
	// the thread query (not scanned); shared with GetThreadUpdates.
	ReadStates []MemberReadState `json:"read_states,omitempty" db:"-"`

	events []event.Base `db:"-"`
}

// ThreadSource is the provider gate of a thread (id from thread_dialog.via, display data from im_thread.source_ref).
type ThreadSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ThreadQueue is the display data of a call_center queue (from im_thread.queue_ref).
type ThreadQueue struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// MemberReadState is one member's delivery/read horizon (per-thread message seq).
// Shared between thread search and the catch-up journal.
type MemberReadState struct {
	MemberID         uuid.UUID `db:"member_id"`
	ThreadID         uuid.UUID `db:"thread_id"`
	DeliveredUpToSeq int64     `db:"delivered_up_to_seq"`
	ReadUpToSeq      int64     `db:"read_up_to_seq"`
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
