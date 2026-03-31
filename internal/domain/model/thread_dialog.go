package model

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type MemberRole int

const (
	RoleMember MemberRole = iota
	RoleAdmin
	RoleSupervisor
	RoleOwner
)

type (
	ThreadDialog struct {
		shared.BaseModel

		MemberID    uuid.UUID         `json:"member_id" db:"member_id"`
		ThreadID    uuid.UUID         `json:"thread_id" db:"thread_id"`
		MemberOf    *uuid.UUID        `json:"member_of" db:"member_of"`
		DirectTo    *uuid.UUID        `json:"direct_to" db:"direct_to"`
		ThreadRole  MemberRole        `json:"member_role" db:"thread_role"`
		Permissions ThreadPermissions `json:"permissions" db:"permissions"`
		Settings    BaseThreadSetting `json:"settings" db:"settings"`
	}

	DirectThreadDialog struct {
		ThreadDialog

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
	Role     MemberRole
	Settings *DirectThreadSetting
}

type ThreadDialogStoreFilter struct {
	Limit    int
	Offset   int
	ThreadID *uuid.UUID
	MemberID *uuid.UUID
	IDs      []uuid.UUID
}
