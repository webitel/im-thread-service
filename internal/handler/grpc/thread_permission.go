package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
)

var _ impb.ThreadPermissionManagementServer = &ThreadPermissionManagementServer{}

type ThreadPermissionManagementService interface {
	Get(ctx context.Context, req *model.GetThreadPermissionRequest) ([]*model.ThreadPermission, error)
	Update(ctx context.Context, req *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error)
}

type ThreadPermissionManagementServer struct {
	impb.UnimplementedThreadPermissionManagementServer

	handler      ThreadPermissionManagementService
	outConverter *mapper.ThreadPermissionOutConverter
	inConverter  *mapper.ThreadPermissionInConverter
}

func NewThreadPermissionServer(threadManager ThreadPermissionManagementService) *ThreadPermissionManagementServer {
	return &ThreadPermissionManagementServer{
		handler:      threadManager,
		inConverter:  &mapper.ThreadPermissionInConverter{},
		outConverter: &mapper.ThreadPermissionOutConverter{},
	}
}

func (t *ThreadPermissionManagementServer) GetThreadPermissions(ctx context.Context, req *impb.GetThreadPermissionsRequest) (*impb.GetThreadPermissionsResponse, error) {
	converted, err := t.inConverter.ConvertGetThreadPermissionRequest(req)
	if err != nil {
		return nil, err
	}

	permissions, err := t.handler.Get(ctx, converted)
	if err != nil {
		return nil, err
	}

	result := t.outConverter.ConvertThreadPermissions(permissions)

	return &impb.GetThreadPermissionsResponse{
		Permissions: result,
	}, nil
}

func (t *ThreadPermissionManagementServer) UpdateThreadPermissions(ctx context.Context, req *impb.UpdateThreadPermissionsRequest) (*impb.ThreadPermissions, error) {
	converted, err := t.inConverter.ConvertUpdateThreadPermissionRequest(req)
	if err != nil {
		return nil, err
	}

	permission, err := t.handler.Update(ctx, converted)
	if err != nil {
		return nil, err
	}

	result := t.outConverter.ConvertThreadPermission(permission)

	return result, nil
}
