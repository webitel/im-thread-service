package mapper

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type ThreadPermissionInConverter struct {
}

func (s *ThreadPermissionInConverter) ConvertGetThreadPermissionRequest(in *impb.GetThreadPermissionsRequest) (*model.GetThreadPermissionRequest, error) {
	if in == nil {
		return nil, nil
	}
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, err
	}
	initiatorID, err := uuid.Parse(in.GetRequestInitiatorId())
	if err != nil {
		return nil, err
	}
	newMemberID, err := uuid.Parse(in.GetMemberId())
	if err != nil {
		return nil, err
	}
	converted := &model.GetThreadPermissionRequest{
		ThreadID:           threadID,
		RequestInitiatorID: initiatorID,
		MemberID:           newMemberID,
	}
	return converted, nil
}

func (s *ThreadPermissionInConverter) ConvertUpdateThreadPermissionRequest(in *impb.UpdateThreadPermissionsRequest) (*model.UpdateThreadPermissionRequest, error) {
	if in == nil {
		return nil, nil
	}
	initiatorID, err := uuid.Parse(in.GetRequestInitiatorId())
	if err != nil {
		return nil, err
	}
	targetMemberID, err := uuid.Parse(in.GetMemberId())
	if err != nil {
		return nil, err
	}
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, err
	}
	converted := &model.UpdateThreadPermissionRequest{
		InitiatorMemberID: initiatorID,
		TargetMemberID:    targetMemberID,
		ThreadID:          threadID,

		CanAddMembers:               in.CanAddMembers,
		CanChangeMembersPermissions: in.CanChangeMembersPermissions,
		CanChangeThreadInfo:         in.CanUpdateThreadInfo,
		CanRemoveMembers:            in.CanRemoveMembers,
		CanSendMessages:             in.CanSendMessages,
	}
	return converted, nil
}

type ThreadPermissionOutConverter struct {
}

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
		Id:                          in.ID.String(),
		ThreadDialogId:              in.ThreadDialogID.String(),
		ThreadId:                    in.ThreadID.String(),
		MemberId:                    in.MemberID.String(),
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
