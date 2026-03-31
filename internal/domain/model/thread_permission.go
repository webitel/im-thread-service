package model

import (
	"time"

	"github.com/google/uuid"
)

type OffsetPagination struct {
	Size int64
	Page int64
}

type GetThreadPermissionRequest struct {
	OffsetPagination
	ThreadDialogID uuid.UUID
}

type ThreadPermissions struct {
	CanSendMessages             bool `db:"can_send_messages"`
	CanAddMembers               bool `db:"can_add_members"`
	CanRemoveMembers            bool `db:"can_remove_members"`
	CanChangeMembersPermissions bool `db:"can_change_members_permissions"`
	CanChangeThreadInfo         bool `db:"can_change_thread_info"`
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
	Role           MemberRole
}

type PermissionChangeTarget struct {
	ThreadDialogID uuid.UUID
	Role           MemberRole
}

type UpdateThreadPermissionRequest struct {
	Initiator *PermissionChangeInitiator
	Target    *PermissionChangeTarget

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
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}
