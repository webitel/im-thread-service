package model

import (
	"time"

	"github.com/google/uuid"
)

type OffsetPagination struct {
	Size   int64
	Offset int64
}

type GetThreadPermissionRequest struct {
	RequestInitiatorID *uuid.UUID
	ThreadID           uuid.UUID
	MemberID           uuid.UUID
}

type ThreadPermissions struct {
	CanSendMessages             bool `db:"can_send_messages"`
	CanAddMembers               bool `db:"can_add_members"`
	CanRemoveMembers            bool `db:"can_remove_members"`
	CanChangeMembersPermissions bool `db:"can_change_members_permissions"`
	CanChangeThreadInfo         bool `db:"can_change_thread_info"`
}

type ThreadPermissionStoreFilters struct {
	OffsetPagination

	ThreadID  *uuid.UUID
	MemberIDs []uuid.UUID
}

type ThreadPermissionsAllowance struct {
	CanSendMessages             bool
	CanAddMembers               bool
	CanRemoveMembers            bool
	CanChangeMembersPermissions bool
	CanChangeThreadInfo         bool
}

type PermissionChangeInitiator struct {
	ThreadPermissions
	ThreadDialogID uuid.UUID
	Role           ThreadRole
}

type PermissionChangeTarget struct {
	ThreadDialogID uuid.UUID
	Role           ThreadRole
}

type UpdateThreadPermissionRequest struct {
	InitiatorContactID *uuid.UUID
	TargetMemberID     uuid.UUID
	ThreadID           uuid.UUID

	CanSendMessages             *bool
	CanAddMembers               *bool
	CanRemoveMembers            *bool
	CanChangeMembersPermissions *bool
	CanChangeThreadInfo         *bool
}

type ThreadPermission struct {
	ThreadPermissions

	ID             uuid.UUID `db:"id"`
	ThreadID       uuid.UUID `db:"thread_id"`
	ThreadDialogID uuid.UUID `db:"thread_dialog_id"`
	MemberID       uuid.UUID `db:"member_id"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}
