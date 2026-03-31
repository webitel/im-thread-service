package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func ptr[T any](v T) *T {
	return &v
}

func Test_checkForSelfPermissionChange(t *testing.T) {
	initiatorUUID := uuid.New()
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		req     *model.UpdateThreadPermissionRequest
		wantErr bool
	}{
		{
			name: "initiator tries to change own permissions ",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleMember,
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleMember,
				},
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change self permissions ",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleMember,
					ThreadPermissions: model.ThreadPermissions{
						CanChangeMembersPermissions: true,
					},
				},
				Target: &model.PermissionChangeTarget{
					Role:           model.RoleMember,
					ThreadDialogID: initiatorUUID,
				},
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change permissions of another member ",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleMember,
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: uuid.New(),
					Role:           model.RoleMember,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkForSelfPermissionChange(tt.req)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("checkForSelfPermissionChange() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("checkForSelfPermissionChange() succeeded unexpectedly")
			}
		})
	}
}

func Test_checkForDownRoleHierarchy(t *testing.T) {
	initiatorUUID := uuid.New()
	targetUUID := uuid.New()
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		req     *model.UpdateThreadPermissionRequest
		wantErr bool
	}{
		{
			name: "initiator tries to change target with equal role ",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleAdmin,
					ThreadPermissions: model.ThreadPermissions{
						CanChangeMembersPermissions: true,
					},
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: targetUUID,
					Role:           model.RoleAdmin,
				},
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change permissions for target with higher role ",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleAdmin,
					ThreadPermissions: model.ThreadPermissions{
						CanChangeMembersPermissions: true,
					},
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: targetUUID,
					Role:           model.RoleOwner,
				},
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change permissions for target with lower role ",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleAdmin,
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: targetUUID,
					Role:           model.RoleMember,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkForDownRoleHierarchy(tt.req)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("checkForDownRoleHierarchy() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("checkForDownRoleHierarchy() succeeded unexpectedly")
			}
		})
	}
}

func Test_checkForPermissionToChangeMembersPermissions(t *testing.T) {
	initiatorUUID := uuid.New()
	targetUUID := uuid.New()
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		req     *model.UpdateThreadPermissionRequest
		wantErr bool
	}{
		{
			name: "initiator does not have permission to change members permissions",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleAdmin,
					ThreadPermissions: model.ThreadPermissions{
						CanChangeMembersPermissions: false,
					},
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: targetUUID,
					Role:           model.RoleMember,
				},
			},
			wantErr: true,
		},
		{
			name: "initiator has permission to change members permissions",
			req: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadDialogID: initiatorUUID,
					Role:           model.RoleAdmin,
					ThreadPermissions: model.ThreadPermissions{
						CanChangeMembersPermissions: true,
					},
				},
				Target: &model.PermissionChangeTarget{
					ThreadDialogID: targetUUID,
					Role:           model.RoleMember,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkForPermissionToChangeMembersPermissions(tt.req)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("checkForPermissionToChangeMembersPermissions() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("checkForPermissionToChangeMembersPermissions() succeeded unexpectedly")
			}
		})
	}
}

func Test_checkPermissionChangeAllowedByTargetRole(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		changes *model.UpdateThreadPermissionRequest
		wantErr bool
	}{
		{
			name:    "nil changes",
			changes: nil,
			wantErr: true,
		},
		{
			name: "nil target",
			changes: &model.UpdateThreadPermissionRequest{
				Target: nil,
			},
			wantErr: true,
		},
		{
			name: "target role does not allow changing this permission",
			changes: &model.UpdateThreadPermissionRequest{
				Target: &model.PermissionChangeTarget{
					Role: model.RoleMember,
				},
				CanRemoveMembers: ptr(true),
			},
			wantErr: true,
		},
		{
			name: "target role allows changing this permission",
			changes: &model.UpdateThreadPermissionRequest{
				Target: &model.PermissionChangeTarget{
					Role: model.RoleOwner,
				},
				CanRemoveMembers: ptr(true),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkPermissionChangeAllowedByTargetRole(tt.changes)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("checkPermissionChangeAllowedByTargetRole() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("checkPermissionChangeAllowedByTargetRole() succeeded unexpectedly")
			}
		})
	}
}

func Test_checkInitiatorHasSamePermissionThatChanged(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		changes *model.UpdateThreadPermissionRequest
		wantErr bool
	}{
		{
			name:    "nil changes",
			changes: nil,
			wantErr: true,
		},
		{
			name: "nil initiator",
			changes: &model.UpdateThreadPermissionRequest{
				Initiator: nil,
			},
			wantErr: true,
		},
		{
			name: "initiator does not have the same permission that is requested to change",
			changes: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadPermissions: model.ThreadPermissions{
						CanRemoveMembers: false,
					},
				},
				CanRemoveMembers: ptr(true),
			},
			wantErr: true,
		},
		{
			name: "initiator has the same permission that is requested to change",
			changes: &model.UpdateThreadPermissionRequest{
				Initiator: &model.PermissionChangeInitiator{
					ThreadPermissions: model.ThreadPermissions{
						CanRemoveMembers: true,
					},
				},
				CanRemoveMembers: ptr(false),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkInitiatorHasSamePermissionThatChanged(tt.changes)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("checkInitiatorHasSamePermissionThatChanged() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("checkInitiatorHasSamePermissionThatChanged() succeeded unexpectedly")
			}
		})
	}
}
