package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
)

var _ impb.ThreadTagManagementServer = &ThreadTagManagementServer{}

type ThreadTagService interface {
	AddTag(ctx context.Context, req *dto.AddTagRequest) (*model.ThreadTag, error)
	RemoveTag(ctx context.Context, req *dto.RemoveTagRequest) error
	SearchTags(ctx context.Context, req *dto.SearchTagsRequest) ([]string, error)
}

type ThreadTagManagementServer struct {
	impb.UnimplementedThreadTagManagementServer

	svc ThreadTagService
}

func NewThreadTagServer(svc ThreadTagService) *ThreadTagManagementServer {
	return &ThreadTagManagementServer{
		svc: svc,
	}
}

func (s *ThreadTagManagementServer) AddTag(ctx context.Context, req *impb.AddThreadTagRequest) (*impb.ChatTag, error) {
	converted, err := mapper.ConvertAddThreadTagRequest(req)
	if err != nil {
		return nil, err
	}

	tag, err := s.svc.AddTag(ctx, converted)
	if err != nil {
		return nil, err
	}

	return mapper.ConvertToChatTag(tag), nil
}

func (s *ThreadTagManagementServer) RemoveTag(ctx context.Context, req *impb.RemoveThreadTagRequest) (*impb.RemoveThreadTagResponse, error) {
	converted, err := mapper.ConvertRemoveThreadTagRequest(req)
	if err != nil {
		return nil, err
	}

	err = s.svc.RemoveTag(ctx, converted)
	if err != nil {
		return nil, err
	}

	return &impb.RemoveThreadTagResponse{}, nil
}

func (s *ThreadTagManagementServer) Search(ctx context.Context, req *impb.SearchThreadTagsRequest) (*impb.SearchThreadTagsResponse, error) {
	converted, err := mapper.ConvertSearchThreadTagsRequest(req)
	if err != nil {
		return nil, err
	}

	tags, err := s.svc.SearchTags(ctx, converted)
	if err != nil {
		return nil, err
	}

	next, tags := utils.ProcessPagination(int(req.GetSize()), tags)

	return &impb.SearchThreadTagsResponse{
		Tags: tags,
		Next: next,
	}, nil
}
