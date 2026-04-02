package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper/generated"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

var (
	s impb.ThreadManagementServer = &ThreadManagementServer{}
)

type ThreadManagementService interface {
	Search(ctx context.Context, searchRequest *dto.ThreadSearchRequest) ([]*model.Thread, error)
	EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error)
	AddMember(context.Context, *dto.AddMemberRequest) error
	RemoveMember(context.Context, *dto.RemoveMemberRequest) error
}

type (
	ThreadManagementServer struct {
		impb.UnimplementedThreadManagementServer

		inMapper      mapper.ThreadInConverter
		outMapper     mapper.ThreadOutConverter
		threadManager ThreadManagementService
	}
)

func NewThreadService(threadManager ThreadManagementService) *ThreadManagementServer {
	return &ThreadManagementServer{
		threadManager: threadManager,
		inMapper:      &generated.ThreadInConverterImpl{},
		outMapper:     &generated.ThreadOutConverterImpl{},
	}
}

func (ts *ThreadManagementServer) Search(ctx context.Context, req *impb.ThreadSearchRequest) (*impb.SearchThreadResponse, error) {
	search, err := ts.inMapper.ConvertSearch(req)
	if err != nil {
		return nil, err
	}

	threads, err := ts.threadManager.Search(ctx, search)
	if err != nil {
		return nil, err
	}

	next, threads := utils.ProcessPagination(int(req.Size), threads)

	var (
		res = impb.SearchThreadResponse{Next: next}
	)

	for _, threadModel := range threads {
		res.Items = append(res.Items, ts.outMapper.ConvertToThread(threadModel))
	}

	return &res, nil
}

// AddMember implements [thread.ThreadManagementServer].
func (ts *ThreadManagementServer) AddMember(ctx context.Context, request *impb.AddMemberRequest) (*impb.AddMemberResponse, error) {
	return nil, nil
}

func (ts *ThreadManagementServer) RemoveMember(ctx context.Context, request *impb.RemoveMemberRequest) (*impb.RemoveMemberResponse, error) {

	return nil, nil
}

// CreateGroup implements [thread.ThreadManagementServer].
func (ts *ThreadManagementServer) CreateGroup(ctx context.Context, request *impb.CreateGroupRequest) (*impb.Thread, error) {
	// internalRequest := ts.inMapper.ConvertCreateGroup(request)

	return nil, errors.Internal("method not implemented yet")
}
