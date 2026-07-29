package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

var (
	allowedPermissionsByRole = map[model.ThreadRole]*model.ThreadPermissionsAllowance{
		model.RoleOwner: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
			CanDeleteMessages:           true,
		},
		model.RoleAdmin: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
			CanDeleteMessages:           true,
		},
		model.RoleSupervisor: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
			CanDeleteMessages:           true,
		},
		model.RoleMember: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
			CanDeleteMessages:           true,
		},
	}
	defaultPermissionsByRole = map[model.ThreadRole]*model.ThreadPermissions{
		model.RoleOwner: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
			CanDeleteMessages:           true,
		},
		model.RoleAdmin: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: true,
			CanChangeThreadInfo:         true,
			CanDeleteMessages:           true,
		},
		model.RoleSupervisor: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
			CanDeleteMessages:           true,
		},
		model.RoleMember: {
			CanSendMessages:             true,
			CanAddMembers:               true,
			CanRemoveMembers:            true,
			CanChangeMembersPermissions: false,
			CanChangeThreadInfo:         false,
			CanDeleteMessages:           true,
		},
	}
)

type ThreadPermissionService struct {
	store  store.UnitOfWork
	logger *slog.Logger
}

func NewThreadPermissionService(store store.UnitOfWork, logger *slog.Logger) (*ThreadPermissionService, error) {
	return &ThreadPermissionService{
		store:  store,
		logger: logger,
	}, nil
}

func (s *ThreadPermissionService) Get(ctx context.Context, req *model.GetThreadPermissionRequest) ([]*model.ThreadPermission, error) {
	if req == nil {
		return nil, errors.InvalidArgument("invalid permission get request")
	}

	if req.RequestInitiatorID != nil {
		initiator, target, err := s.store.ThreadDialogStore().FindActorsPair(ctx, *req.RequestInitiatorID, req.MemberID)
		if err != nil {
			return nil, err
		}

		if initiator == nil {
			return nil, errors.New("request initiator is not a member of the thread")
		}

		if target == nil {
			return nil, errors.New("target member is not a member of the thread")
		}

		if initiator.ThreadRole < target.ThreadRole {
			return nil, errors.New("request initiator does not have enough role permissions to get target permissions")
		}
	}

	return s.store.ThreadPermissionStore().Get(ctx, &model.ThreadPermissionStoreFilters{
		MemberIDs: []uuid.UUID{req.MemberID},
	})
}

func (s *ThreadPermissionService) Update(ctx context.Context, req *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error) {
	if req == nil {
		return nil, errors.InvalidArgument("invalid permission change request")
	}

	if req.InitiatorContactID != nil {
		initiator, target, err := s.store.ThreadDialogStore().FindActorsPair(ctx, *req.InitiatorContactID, req.TargetMemberID)
		if err != nil {
			return nil, err
		}

		if initiator == nil {
			return nil, errors.New("request initiator is not a member of the thread")
		}

		if target == nil {
			return nil, errors.New("target member is not a member of the thread")
		}

		validationStruct := &permissionChangeValidationStruct{
			Initiator: initiator,
			Target:    target,
			Changes:   req,
		}

		if err := s.validatePermissionChangeRequest(validationStruct); err != nil {
			return nil, err
		}
	}

	return s.store.ThreadPermissionStore().Update(ctx, req)
}

type permissionChangeValidationStruct struct {
	Initiator *model.ThreadDialogExtended
	Target    *model.ThreadDialogExtended

	Changes *model.UpdateThreadPermissionRequest
}

func (s *ThreadPermissionService) validatePermissionChangeRequest(req *permissionChangeValidationStruct) error {
	if req == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	checks := []func(*permissionChangeValidationStruct) error{
		checkForSelfPermissionChange,
		checkForDownRoleHierarchy,
		checkForPermissionToChangeMembersPermissions,
		checkInitiatorHasSamePermissionThatChanged,
		checkPermissionChangeAllowedByTargetRole,
	}

	for _, check := range checks {
		if err := check(req); err != nil {
			return err
		}
	}

	return nil
}

func checkForSelfPermissionChange(req *permissionChangeValidationStruct) error {
	if req == nil || req.Initiator == nil || req.Target == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	initiator, target := req.Initiator, req.Target
	if initiator.ID == target.ID {
		return errors.New("can't change self permissions")
	}

	return nil
}

func checkForDownRoleHierarchy(req *permissionChangeValidationStruct) error {
	if req == nil || req.Initiator == nil || req.Target == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	initiator, target := req.Initiator, req.Target
	if initiator.ThreadRole <= target.ThreadRole {
		return errors.New("initiator does not have enough role permissions to change target")
	}

	return nil
}

func checkForPermissionToChangeMembersPermissions(req *permissionChangeValidationStruct) error {
	if req == nil || req.Initiator == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	initiator := req.Initiator
	if !initiator.Permissions.CanChangeMembersPermissions {
		return errors.New("initiator does not have permission to change members permissions")
	}

	return nil
}

func checkPermissionChangeAllowedByTargetRole(req *permissionChangeValidationStruct) error {
	if req == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	if req.Changes == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	if req.Target == nil {
		return errors.InvalidArgument("permission change target cannot be nil")
	}

	allowed, err := getAllowedPermissionsByRole(req.Target.ThreadRole)
	if err != nil {
		return errors.New("failed to get allowed permissions by target role", errors.WithCause(err))
	}

	changes := req.Changes
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
		{changes.CanDeleteMessages, allowed.CanDeleteMessages, "change delete messages permission is not allowed for the target role"},
	}

	for _, rule := range rules {
		if rule.requested != nil && !rule.allowed {
			return errors.New(rule.errMsg)
		}
	}

	return nil
}

func getAllowedPermissionsByRole(role model.ThreadRole) (*model.ThreadPermissionsAllowance, error) {
	allowed, ok := allowedPermissionsByRole[role]
	if !ok {
		return nil, errors.New("unknown member role")
	}

	return allowed, nil
}

func getDefaultPermissionsByRole(role model.ThreadRole) (*model.ThreadPermissions, error) {
	defaultPermissions, ok := defaultPermissionsByRole[role]
	if !ok {
		return nil, errors.New("unknown member role")
	}

	return defaultPermissions, nil
}

func checkInitiatorHasSamePermissionThatChanged(req *permissionChangeValidationStruct) error {
	if req == nil || req.Initiator == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	if req.Changes == nil {
		return errors.InvalidArgument("invalid permission change request")
	}

	initiator := req.Initiator.Permissions
	changes := req.Changes

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
		{changes.CanDeleteMessages, initiator.CanDeleteMessages, "initiator does not have permission to change delete messages permission"},
	}

	for _, rule := range rules {
		if rule.requested != nil && !rule.allowed {
			return errors.New(rule.errMsg)
		}
	}

	return nil
}
