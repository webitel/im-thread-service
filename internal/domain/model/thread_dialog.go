package model

import (
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

type (
	ThreadDialogExtended struct {
		shared.BaseModel

		MemberID   uuid.UUID  `json:"member_id" db:"member_id"`
		ThreadID   uuid.UUID  `json:"thread_id" db:"thread_id"`
		MemberOf   *uuid.UUID `json:"member_of" db:"member_of"`
		DirectTo   *uuid.UUID `json:"direct_to" db:"direct_to"`
		ThreadRole ThreadRole `json:"member_role" db:"thread_role"`

		Permissions ThreadPermissions `json:"permissions" db:"permissions"`
		Settings    BaseThreadSetting `json:"settings" db:"settings"`
	}
	ThreadDialog struct {
		shared.BaseModel

		MemberID   uuid.UUID  `json:"member_id" db:"member_id"`
		ThreadID   uuid.UUID  `json:"thread_id" db:"thread_id"`
		MemberOf   *uuid.UUID `json:"member_of" db:"member_of"`
		DirectTo   *uuid.UUID `json:"direct_to" db:"direct_to"`
		ThreadRole ThreadRole `json:"member_role" db:"thread_role"`
	}

	DirectThreadDialog struct {
		ThreadDialogExtended

		Settings *DirectThreadSetting
	}
)

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
	Limit     int
	Offset    int
	ThreadIDs []uuid.UUID
	MemberIDs []uuid.UUID
	IDs       []uuid.UUID
}
