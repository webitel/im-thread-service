package grpc

import (
	"context"

	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

var (
	_ impb.ThreadManagementServer = (*ThreadManagementServer)(nil)
)

type ThreadManagementService interface {
	Search(ctx context.Context, searchRequest *dto.ThreadSearchRequest) ([]*model.Thread, error)
	AddMember(context.Context, *dto.AddMemberRequest) (uuid.UUID, error)
	RemoveMember(context.Context, *dto.RemoveMemberRequest) error
}

type ThreadVariablesOperator interface {
	Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error)
	Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error)
	Locate(ctx context.Context, threadID uuid.UUID) (*model.ThreadVariables, error)
	Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error)
}

type (
	ThreadManagementServer struct {
		impb.UnimplementedThreadManagementServer

		inMapper        *mapper.ThreadInConverter
		outMapper       *mapper.ThreadOutConverter
		threadManager   ThreadManagementService
		threadVariables ThreadVariablesOperator
	}
)

func NewThreadService(threadManager ThreadManagementService, threadVariables ThreadVariablesOperator) *ThreadManagementServer {
	return &ThreadManagementServer{
		threadManager:   threadManager,
		inMapper:        &mapper.ThreadInConverter{},
		outMapper:       &mapper.ThreadOutConverter{},
		threadVariables: threadVariables,
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
	internalRequest, err := ts.inMapper.ConvertAddMemberRequest(request)
	if err != nil {
		return nil, err
	}

	newMember, err := ts.threadManager.AddMember(ctx, internalRequest)
	if err != nil {
		return nil, err
	}

	return &impb.AddMemberResponse{Member: &impb.ThreadMember{
		Id: newMember.String(),
	}}, nil
}

func (ts *ThreadManagementServer) RemoveMember(ctx context.Context, request *impb.RemoveMemberRequest) (*impb.RemoveMemberResponse, error) {
	internalRequest, err := ts.inMapper.ConvertRemoveMemberRequest(request)
	if err != nil {
		return nil, err
	}

	err = ts.threadManager.RemoveMember(ctx, internalRequest)
	if err != nil {
		return nil, err
	}

	return &impb.RemoveMemberResponse{}, nil
}

// CreateGroup implements [thread.ThreadManagementServer].
func (ts *ThreadManagementServer) CreateGroup(ctx context.Context, request *impb.CreateGroupRequest) (*impb.Thread, error) {
	// internalRequest := ts.inMapper.ConvertCreateGroup(request)

	return nil, errors.Internal("method not implemented yet")
}

func (ts *ThreadManagementServer) SetVariables(ctx context.Context, req *impb.SetVariablesRequest) (*impb.ThreadVariables, error) {
	setVarsCmd, err := mapper.MapSetVariablesRequestToCommand(req)
	if err != nil {
		return nil, err
	}

	settedVars, err := ts.threadVariables.Set(ctx, setVarsCmd)
	if err != nil {
		return nil, err
	}

	return mapper.MapThreadVariablesToProto(settedVars), nil
}

func (ts *ThreadManagementServer) SearchVariables(ctx context.Context, req *impb.SearchVariablesRequest) (*impb.SearchVariablesResponse, error) {
	searchQuery, err := mapper.MapSearchVariablesRequestToQuery(req)
	if err != nil {
		return nil, err
	}

	page, err := ts.threadVariables.Search(ctx, searchQuery)
	if err != nil {
		return nil, err
	}

	return mapper.MapThreadVariablesPageToProto(&page), nil
}

func (ts *ThreadManagementServer) LocateVariables(ctx context.Context, req *impb.LocateVariablesRequest) (*impb.ThreadVariables, error) {
	threadId, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread id format", errors.WithCause(err))
	}

	vars, err := ts.threadVariables.Locate(ctx, threadId)
	if err != nil {
		return nil, err
	}

	return mapper.MapThreadVariablesToProto(vars), nil
}

func (ts *ThreadManagementServer) FlushVariables(ctx context.Context, req *impb.FlushVariablesRequest) (*impb.ThreadVariables, error) {
	cmd, err := mapper.MapFlushVariablesRequestToCommand(req)
	if err != nil {
		return nil, err
	}

	vars, err := ts.threadVariables.Flush(ctx, *cmd)
	if err != nil {
		return nil, err
	}

	return mapper.MapThreadVariablesToProto(vars), nil
}
