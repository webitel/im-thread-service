package model

import (
	"time"

	"github.com/google/uuid"
)

type MessageRevisionAction int16

const (
	MessageRevisionActionUnknown MessageRevisionAction = iota
	MessageRevisionActionCreated
	MessageRevisionActionEdited
	MessageRevisionActionDeleted
)

// MessageRevision is one entry of a message's change history: the body the
// message carried at that point, plus who changed it and when. Entries are
// append-only and are never rewritten by later changes.
type MessageRevision struct {
	MessageID uuid.UUID             `json:"message_id" db:"message_id"`
	DomainID  int32                 `json:"domain_id" db:"domain_id"`
	Version   int32                 `json:"version" db:"version"`
	Action    MessageRevisionAction `json:"action" db:"action"`
	Body      string                `json:"body" db:"body"`
	ChangedBy uuid.UUID             `json:"changed_by" db:"changed_by"`
	ChangedAt time.Time             `json:"changed_at" db:"changed_at"`
}

func (r *MessageRevision) ChangedAtUnixMillis() int64 {
	if r == nil {
		return 0
	}

	return max(r.ChangedAt.UTC().UnixMilli(), 0)
}
