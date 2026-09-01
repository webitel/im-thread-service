package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// Covers CW-60 Bug 3: an owner must be able to release a bot even though the bot
// is also RoleOwner in a bot-control thread (equal role), while human-to-human
// removal keeps the strict "must outrank" rule.
func TestVerifyRemoveMemberInitiatorPermissions(t *testing.T) {
	canRemove := &model.ThreadPermissions{CanRemoveMembers: true}
	cannotRemove := &model.ThreadPermissions{CanRemoveMembers: false}

	svc := &ThreadManagementService{}

	tests := []struct {
		name          string
		initiatorRole model.ThreadRole
		targetRole    model.ThreadRole
		perms         *model.ThreadPermissions
		targetIsBot   bool
		wantErr       bool
	}{
		{"nil permissions", model.RoleOwner, model.RoleMember, nil, false, true},
		{"no CanRemoveMembers", model.RoleOwner, model.RoleMember, cannotRemove, false, true},

		// Human targets: strict hierarchy.
		{"human outranks target", model.RoleOwner, model.RoleMember, canRemove, false, false},
		{"human equal role forbidden", model.RoleOwner, model.RoleOwner, canRemove, false, true},
		{"human lower role forbidden", model.RoleMember, model.RoleOwner, canRemove, false, true},

		// Bot targets: equal-or-higher allowed (the fix).
		{"owner releases owner bot", model.RoleOwner, model.RoleOwner, canRemove, true, false},
		{"admin releases admin bot", model.RoleAdmin, model.RoleAdmin, canRemove, true, false},
		{"owner releases lower bot", model.RoleOwner, model.RoleMember, canRemove, true, false},
		{"member cannot release owner bot", model.RoleMember, model.RoleOwner, canRemove, true, true},
		{"bot target still needs CanRemoveMembers", model.RoleOwner, model.RoleOwner, cannotRemove, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.verifyRemoveMemberInitiatorPermissions(tt.initiatorRole, tt.targetRole, tt.perms, tt.targetIsBot)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
