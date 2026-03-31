package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

var (
	allowedPermissionsByRole = map[model.MemberRole]*model.ThreadPermissionsAllowance{
		model.RoleOwner: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
		},
		model.RoleAdmin: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
		},
		model.RoleSupervisor: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
		},
		model.RoleMember: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
		},
	}
	defaultPermissionsByRole = map[model.MemberRole]*model.ThreadPermissions{
		model.RoleOwner: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
		},
		model.RoleAdmin: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
		},
		model.RoleSupervisor: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
		},
		model.RoleMember: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
		},
	}
)

//mockery:generate: true
type ThreadPermissionStore interface {
	Create(ctx context.Context, req *model.ThreadPermission) (*model.ThreadPermission, error)
	Get(ctx context.Context, req *model.GetThreadPermissionRequest) (*model.ThreadPermission, error)
	Update(ctx context.Context, req *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error)
}

type ThreadPermissionService struct {
	store  ThreadPermissionStore
	logger *slog.Logger
}

func NewThreadPermissionService(store ThreadPermissionStore, logger *slog.Logger) (*ThreadPermissionService, error) {
	return &ThreadPermissionService{
		store:  store,
		logger: logger,
	}, nil
}

func (s *ThreadPermissionService) Get(ctx context.Context, req *model.GetThreadPermissionRequest) (*model.ThreadPermission, error) {
	if req == nil {
		return nil, errors.InvalidArgument("invalid permission get request")
	}
	if req.ThreadDialogID == uuid.Nil {
		return nil, errors.InvalidArgument("thread dialog ID cannot be empty")
	}

	return s.store.Get(ctx, req)
}

func (s *ThreadPermissionService) Update(ctx context.Context, req *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error) {
	if err := s.validatePermissionChangeRequest(req); err != nil {
		return nil, err
	}
	return s.store.Update(ctx, req)
}

func (s *ThreadPermissionService) validatePermissionChangeRequest(req *model.UpdateThreadPermissionRequest) error {
	var (
		checks = []func(*model.UpdateThreadPermissionRequest) error{
			checkForSelfPermissionChange,
			checkForDownRoleHierarchy,
			checkForPermissionToChangeMembersPermissions,
			checkInitiatorHasSamePermissionThatChanged,
			checkPermissionChangeAllowedByTargetRole,
		}
	)

	for _, check := range checks {
		if err := check(req); err != nil {
			return errors.New("permission change request failed validation", errors.WithCause(err))
		}
	}
	return nil
}

func checkForSelfPermissionChange(req *model.UpdateThreadPermissionRequest) error {
	if req == nil || req.Initiator == nil || req.Target == nil {
		return errors.InvalidArgument("invalid permission change request")
	}
	initiator, target := req.Initiator, req.Target
	if initiator.ThreadDialogID == target.ThreadDialogID {
		return errors.New("initiator can only change own permissions in self change scenario")
	}
	return nil
}

func checkForDownRoleHierarchy(req *model.UpdateThreadPermissionRequest) error {
	if req == nil || req.Initiator == nil || req.Target == nil {
		return errors.InvalidArgument("invalid permission change request")
	}
	initiator, target := req.Initiator, req.Target
	if initiator.Role <= target.Role {
		return errors.New("initiator does not have enough role permissions to change target")
	}
	return nil
}

func checkForPermissionToChangeMembersPermissions(req *model.UpdateThreadPermissionRequest) error {
	if req == nil || req.Initiator == nil {
		return errors.InvalidArgument("invalid permission change request")
	}
	initiator := req.Initiator
	if !initiator.CanChangeMembersPermissions {
		return errors.New("initiator does not have permission to change members permissions")
	}
	return nil
}

func checkPermissionChangeAllowedByTargetRole(changes *model.UpdateThreadPermissionRequest) error {
	if changes == nil {
		return errors.InvalidArgument("invalid permission change request")
	}
	if changes.Target == nil {
		return errors.InvalidArgument("permission change target cannot be nil")
	}

	allowed, err := getAllowedPermissionsByRole(changes.Target.Role)
	if err != nil {
		return errors.New("failed to get allowed permissions by target role", errors.WithCause(err))
	}
	rules := []struct {
		requested *bool
		allowed   bool
		errMsg    string
	}{
		{changes.CanSendMessages, allowed.CanSendMessages, "change send messages permission is not allowed for the target role"},
		{changes.CanAddMembers, allowed.CanAddMembers, "change invite members permission is not allowed for the target role"},
		{changes.CanRemoveMembers, allowed.CanRemoveMembers, "change add admins permission is not allowed for the target role"},
		{changes.CanChangeMembersPermissions, allowed.CanChangeMembersPermissions, "change members permissions permission is not allowed for the target role"},
		{changes.CanChangeThreadInfo, allowed.CanChangeThreadInfo, "change thread info permission is not allowed for the target role"},
	}

	for _, rule := range rules {
		if rule.requested != nil && !rule.allowed {
			return errors.New(rule.errMsg)
		}
	}

	return nil

}

func getAllowedPermissionsByRole(role model.MemberRole) (*model.ThreadPermissionsAllowance, error) {
	allowed, ok := allowedPermissionsByRole[role]
	if !ok {
		return nil, errors.New("unknown member role")
	}
	return allowed, nil
}

func getDefaultPermissionsByRole(role model.MemberRole) (*model.ThreadPermissions, error) {
	defaultPermissions, ok := defaultPermissionsByRole[role]
	if !ok {
		return nil, errors.New("unknown member role")
	}
	return defaultPermissions, nil
}
func checkInitiatorHasSamePermissionThatChanged(changes *model.UpdateThreadPermissionRequest) error {
	if changes == nil || changes.Initiator == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	initiator := changes.Initiator

	rules := []struct {
		requested *bool
		allowed   bool
		errMsg    string
	}{
		{changes.CanSendMessages, initiator.CanSendMessages, "initiator does not have permission to change send messages permission"},
		{changes.CanAddMembers, initiator.CanAddMembers, "initiator does not have permission to change invite members permission"},
		{changes.CanRemoveMembers, initiator.CanRemoveMembers, "initiator does not have permission to remove members"},
		{changes.CanChangeMembersPermissions, initiator.CanChangeMembersPermissions, "initiator does not have permission to change members permissions permission"},
		{changes.CanChangeThreadInfo, initiator.CanChangeThreadInfo, "initiator does not have permission to change thread info permission"},
	}

	for _, rule := range rules {
		if rule.requested != nil && !rule.allowed {
			return errors.New(rule.errMsg)
		}
	}

	return nil
}
