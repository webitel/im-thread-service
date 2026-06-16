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

	IsBot       bool `json:"is_bot" db:"is_bot"`
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

	IsBot       bool `json:"is_bot" db:"is_bot"`
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

	var external []*ExternalPeerPair

	for _, threadDialog := range threadDialogs {
		if threadDialog.Via != nil {
			external = append(external, &ExternalPeerPair{
				ContactID: threadDialog.ContactID,
				Via:       *threadDialog.Via,
			})
		}
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
