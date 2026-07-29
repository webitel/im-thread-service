package mapper

import (
	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type ThreadPermissionInConverter struct{}

func (s *ThreadPermissionInConverter) ConvertGetThreadPermissionRequest(in *impb.GetThreadPermissionsRequest) (*model.GetThreadPermissionRequest, error) {
	if in == nil {
		return nil, errors.InvalidArgument("received nil pointer get thread permissions request", errors.WithID("mapper.thread_permissions.convert_get_thread_permission_request"))
	}

	newMemberID, err := uuid.Parse(in.GetMemberId())
	if err != nil {
		return nil, err
	}

	converted := &model.GetThreadPermissionRequest{
		MemberID: newMemberID,
	}

	if in.InitiatorContactId != nil {
		initiatorID, err := uuid.Parse(in.GetInitiatorContactId())
		if err != nil {
			return nil, err
		}

		converted.RequestInitiatorID = &initiatorID
	}

	return converted, nil
}

func (s *ThreadPermissionInConverter) ConvertUpdateThreadPermissionRequest(in *impb.UpdateThreadPermissionsRequest) (*model.UpdateThreadPermissionRequest, error) {
	if in == nil {
		return nil, errors.InvalidArgument(
			"received nil pointer update thread permissions request",
			errors.WithID("mapper.thread_permissions.convert_update_thread_permission_request"),
		)
	}

	targetMemberID, err := uuid.Parse(in.GetMemberId())
	if err != nil {
		return nil, err
	}

	converted := &model.UpdateThreadPermissionRequest{
		TargetMemberID: targetMemberID,

		CanAddMembers:               in.CanAddMembers,
		CanChangeMembersPermissions: in.CanChangeMembersPermissions,
		CanChangeThreadInfo:         in.CanChangeThreadInfo,
		CanDeleteMessages:           in.CanDeleteMessages,
		CanRemoveMembers:            in.CanRemoveMembers,
		CanSendMessages:             in.CanSendMessages,
	}
	if in.InitiatorContactId != nil {
		initiatorID, err := uuid.Parse(in.GetInitiatorContactId())
		if err != nil {
			return nil, err
		}

		converted.InitiatorContactID = &initiatorID
	}

	return converted, nil
}

type ThreadPermissionOutConverter struct{}

func (s *ThreadPermissionOutConverter) ConvertThreadPermission(in *model.ThreadPermission) *impb.ThreadPermissions {
	if in == nil {
		return nil
	}

	return &impb.ThreadPermissions{
		CanSendMessages:             in.CanSendMessages,
		CanAddMembers:               in.CanAddMembers,
		CanRemoveMembers:            in.CanRemoveMembers,
		CanChangeMembersPermissions: in.CanChangeMembersPermissions,
		CanChangeThreadInfo:         in.CanChangeThreadInfo,
		CanDeleteMessages:           in.CanDeleteMessages,
		Id:                          in.ID.String(),
		MemberId:                    in.ThreadDialogID.String(),
		ThreadId:                    in.ThreadID.String(),
		CreatedAt:                   in.CreatedAt.UnixMilli(),
		UpdatedAt:                   in.UpdatedAt.UnixMilli(),
	}
}

func (s *ThreadPermissionOutConverter) ConvertThreadPermissions(in []*model.ThreadPermission) []*impb.ThreadPermissions {
	if in == nil {
		return nil
	}

	out := make([]*impb.ThreadPermissions, len(in))
	for i, permission := range in {
		out[i] = s.ConvertThreadPermission(permission)
	}

	return out
}
