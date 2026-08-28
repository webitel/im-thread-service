package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type ThreadRole int

const (
	UnspecifiedRole ThreadRole = iota
	RoleMember
	RoleAdmin
	RoleSupervisor
	RoleOwner
)

type ThreadDialogExtended struct {
	shared.BaseModel

	ContactID   uuid.UUID  `json:"member_id" db:"member_id"`
	ThreadID    uuid.UUID  `json:"thread_id" db:"thread_id"`
	ThreadRole  ThreadRole `json:"member_role" db:"thread_role"`
	DeletedAt   *time.Time `json:"deleted_at" db:"deleted_at"`
	InvitedBy   *uuid.UUID `json:"invited_by" db:"invited_by"`
	LeaveReason *string    `json:"leave_reason" db:"leave_reason"`
	Via         *string    `json:"via,omitempty" db:"via"`

	IsBot     bool `json:"is_bot" db:"is_bot"`
	AutoLeave bool `json:"auto_leave" db:"auto_leave"`

	Permissions ThreadPermissions `json:"permissions" db:"permissions"`
	Settings    BaseThreadSetting `json:"settings" db:"settings"`
}

type ThreadDialog struct {
	shared.BaseModel

	ContactID   uuid.UUID  `json:"member_id" db:"member_id"`
	ThreadID    uuid.UUID  `json:"thread_id" db:"thread_id"`
	ThreadRole  ThreadRole `json:"member_role" db:"thread_role"`
	DeletedAt   *time.Time `json:"deleted_at" db:"deleted_at"`
	InvitedBy   *uuid.UUID `json:"invited_by" db:"invited_by"`
	LeaveReason *string    `json:"leave_reason" db:"leave_reason"`
	Via         *string    `json:"via,omitempty" db:"via"`

	IsBot     bool `json:"is_bot" db:"is_bot"`
	AutoLeave bool `json:"auto_leave" db:"auto_leave"`
}

type ThreadDialogs []*ThreadDialog

type ExternalPeerPair struct {
	ContactID uuid.UUID
	Via       string
}

func (threadDialogs ThreadDialogs) ExtractExternalPeers() []*ExternalPeerPair {
	if len(threadDialogs) == 0 {
		return nil
	}

	// Via is the gate id, a property of the thread's gate rather than of any
	// single participant. Depending on how the thread was created it may be
	// written to the external contact's row, the bot's row, or both. Recover it
	// from whichever participant carries it so external recipients stay
	// resolvable even when the value never reached their own row.
	gateVia := ""

	nonBots := make([]*ThreadDialog, 0, len(threadDialogs))

	for _, threadDialog := range threadDialogs {
		if threadDialog.Via != nil && *threadDialog.Via != "" && gateVia == "" {
			gateVia = *threadDialog.Via
		}

		// A bot is internal and can never be an external recipient, even if a
		// stale/incorrect via was written to its row. Skipping it here prevents
		// outbound messages from being addressed to the bot's own subject id
		// (e.g. the Facebook gate bot) instead of the customer.
		if !threadDialog.IsBot {
			nonBots = append(nonBots, threadDialog)
		}
	}

	var external []*ExternalPeerPair

	for _, threadDialog := range nonBots {
		if threadDialog.Via != nil && *threadDialog.Via != "" {
			external = append(external, &ExternalPeerPair{
				ContactID: threadDialog.ContactID,
				Via:       *threadDialog.Via,
			})
		}
	}

	// Self-heal a direct thread whose only non-bot participant lost its via:
	// there is exactly one external contact and the gate id is known from the
	// bot's row, so the recipient is unambiguous. Restricted to the single
	// non-bot case to avoid tagging internal agents in group threads.
	if len(external) == 0 && len(nonBots) == 1 && gateVia != "" {
		external = append(external, &ExternalPeerPair{
			ContactID: nonBots[0].ContactID,
			Via:       gateVia,
		})
	}

	return external
}

type DirectThreadDialog struct {
	ThreadDialogExtended

	Settings *DirectThreadSetting
}

type CreateDirectThreadDialogRequest struct {
	DomainID int
	ThreadID uuid.UUID
	From     CreateDirectPeer
	To       CreateDirectPeer
}

type CreateDirectPeer struct {
	ID       uuid.UUID
	Name     string
	Role     ThreadRole
	Settings *DirectThreadSetting
}

type ThreadDialogStoreFilter struct {
	Limit          int
	Offset         int
	ThreadIDs      []uuid.UUID
	ContactIDs     []uuid.UUID
	IDs            []uuid.UUID
	IncludeDeleted bool
}
