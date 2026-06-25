package model

import "github.com/google/uuid"

type BotControlReason string

const (
	BotControlReasonInitial   BotControlReason = "initial"
	BotControlReasonTransfer  BotControlReason = "transfer"
	BotControlReasonCompleted BotControlReason = "completed"
	BotControlReasonRemoved   BotControlReason = "removed"
)

type BotControlTransition struct {
	ThreadID     uuid.UUID
	NewMemberID  uuid.UUID
	PrevMemberID *uuid.UUID
	Reason       BotControlReason
	TriggeredBy  *uuid.UUID
}

type BotControlStackEntry struct {
	ID       uuid.UUID  `db:"id"`
	ThreadID uuid.UUID  `db:"thread_id"`
	MemberID *uuid.UUID `db:"member_id"`
	Position int        `db:"position"`

	// Populated by Pop — carries the dialog context of the new top so callers
	// can publish bot.control.granted.v1 without an extra DB round-trip.
	ContactID uuid.UUID `db:"contact_id"`
	DomainID  int       `db:"domain_id"`
	AutoLeave bool      `db:"auto_leave"`
}

// BotControlPushResult is returned by Push.
// Prev is the stack entry that was top before the push (nil if stack was empty).
type BotControlPushResult struct {
	Prev *BotControlStackEntry
}
