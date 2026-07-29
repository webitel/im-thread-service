package service

import (
	"testing"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func ptr[T any](v T) *T {
	return &v
}

func testDialog(id uuid.UUID, role model.ThreadRole, permissions model.ThreadPermissions) *model.ThreadDialogExtended {
	return &model.ThreadDialogExtended{
		BaseModel:   shared.BaseModel{ID: id},
		ThreadRole:  role,
		Permissions: permissions,
	}
}

func Test_checkForSelfPermissionChange(t *testing.T) {
	initiatorUUID := uuid.New()

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		req     *permissionChangeValidationStruct
		wantErr bool
	}{
		{
			name: "initiator tries to change own permissions ",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleMember, model.ThreadPermissions{}),
				Target:    testDialog(initiatorUUID, model.RoleMember, model.ThreadPermissions{}),
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change self permissions ",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleMember, model.ThreadPermissions{CanChangeMembersPermissions: true}),
				Target:    testDialog(initiatorUUID, model.RoleMember, model.ThreadPermissions{}),
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change permissions of another member ",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleMember, model.ThreadPermissions{}),
				Target:    testDialog(uuid.New(), model.RoleMember, model.ThreadPermissions{}),
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
		req     *permissionChangeValidationStruct
		wantErr bool
	}{
		{
			name: "initiator tries to change target with equal role ",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(targetUUID, model.RoleAdmin, model.ThreadPermissions{CanChangeMembersPermissions: true}),
				Target:    testDialog(targetUUID, model.RoleAdmin, model.ThreadPermissions{}),
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change permissions for target with higher role ",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleAdmin, model.ThreadPermissions{CanChangeMembersPermissions: true}),
				Target:    testDialog(targetUUID, model.RoleOwner, model.ThreadPermissions{}),
			},
			wantErr: true,
		},
		{
			name: "initiator tries to change permissions for target with lower role ",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleAdmin, model.ThreadPermissions{}),
				Target:    testDialog(targetUUID, model.RoleMember, model.ThreadPermissions{}),
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
		req     *permissionChangeValidationStruct
		wantErr bool
	}{
		{
			name: "initiator does not have permission to change members permissions",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleAdmin, model.ThreadPermissions{CanChangeMembersPermissions: false}),
				Target:    testDialog(targetUUID, model.RoleMember, model.ThreadPermissions{}),
			},
			wantErr: true,
		},
		{
			name: "initiator has permission to change members permissions",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(initiatorUUID, model.RoleAdmin, model.ThreadPermissions{CanChangeMembersPermissions: true}),
				Target:    testDialog(targetUUID, model.RoleMember, model.ThreadPermissions{}),
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
		req     *permissionChangeValidationStruct
		wantErr bool
	}{
		{
			name:    "nil changes",
			req:     &permissionChangeValidationStruct{},
			wantErr: true,
		},
		{
			name: "nil target",
			req: &permissionChangeValidationStruct{
				Changes: &model.UpdateThreadPermissionRequest{},
			},
			wantErr: true,
		},
		{
			name: "target role does not allow changing this permission",
			req: &permissionChangeValidationStruct{
				Target: testDialog(uuid.New(), model.RoleMember, model.ThreadPermissions{}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanChangeMembersPermissions: ptr(true),
				},
			},
			wantErr: true,
		},
		{
			name: "target role allows changing this permission",
			req: &permissionChangeValidationStruct{
				Target: testDialog(uuid.New(), model.RoleOwner, model.ThreadPermissions{}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanRemoveMembers: ptr(true),
				},
			},
			wantErr: false,
		},
		{
			name: "delete messages permission may be revoked from a plain member",
			req: &permissionChangeValidationStruct{
				Target: testDialog(uuid.New(), model.RoleMember, model.ThreadPermissions{}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanDeleteMessages: ptr(false),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkPermissionChangeAllowedByTargetRole(tt.req)
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
		req     *permissionChangeValidationStruct
		wantErr bool
	}{
		{
			name:    "nil changes",
			req:     nil,
			wantErr: true,
		},
		{
			name: "nil initiator",
			req: &permissionChangeValidationStruct{
				Changes: &model.UpdateThreadPermissionRequest{},
			},
			wantErr: true,
		},
		{
			name: "initiator does not have the same permission that is requested to change",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(uuid.New(), model.RoleAdmin, model.ThreadPermissions{CanRemoveMembers: false}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanRemoveMembers: ptr(true),
				},
			},
			wantErr: true,
		},
		{
			name: "initiator has the same permission that is requested to change",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(uuid.New(), model.RoleAdmin, model.ThreadPermissions{CanRemoveMembers: true}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanRemoveMembers: ptr(false),
				},
			},
			wantErr: false,
		},
		{
			name: "initiator whose delete messages permission was revoked cannot revoke it from others",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(uuid.New(), model.RoleAdmin, model.ThreadPermissions{CanDeleteMessages: false}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanDeleteMessages: ptr(false),
				},
			},
			wantErr: true,
		},
		{
			name: "initiator holding delete messages permission may change it",
			req: &permissionChangeValidationStruct{
				Initiator: testDialog(uuid.New(), model.RoleAdmin, model.ThreadPermissions{CanDeleteMessages: true}),
				Changes: &model.UpdateThreadPermissionRequest{
					CanDeleteMessages: ptr(false),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := checkInitiatorHasSamePermissionThatChanged(tt.req)
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
