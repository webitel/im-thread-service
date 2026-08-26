package model

import (
	"time"
)

type MessageRevisionAction int16

const (
	MessageRevisionActionUnknown MessageRevisionAction = iota
	MessageRevisionActionCreated
	MessageRevisionActionEdited
	MessageRevisionActionDeleted
)

// MessageChangeEntry is one line of a message's replayed history: what the
// message read at that point, who put it there and when. The log is assembled
// by MessageRevisionStore.Search; only overwritten bodies are stored, so
// creation and deletion entries come from the message row itself.
type MessageChangeEntry struct {
	Version   int32                 `json:"version" db:"version"`
	Action    MessageRevisionAction `json:"action" db:"action"`
	Body      string                `json:"body" db:"body"`
	ChangedBy *ThreadDialog         `json:"changed_by" db:"changed_by"`
	ChangedAt time.Time             `json:"changed_at" db:"changed_at"`
}

func (e *MessageChangeEntry) ChangedAtUnixMillis() int64 {
	if e == nil {
		return 0
	}

	return max(e.ChangedAt.UTC().UnixMilli(), 0)
}
