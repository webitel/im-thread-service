package grpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
)

var _ impb.ThreadManagementServer = (*ThreadManagementServer)(nil)

type ThreadManagementService interface {
	Get(ctx context.Context, req *dto.ThreadGetRequest) (*model.Thread, error)
	Search(ctx context.Context, searchRequest *dto.ThreadSearchRequest) ([]*model.Thread, error)
	SearchLeft(ctx context.Context, req *dto.SearchLeftRequest) ([]*model.Thread, error)
	GetUnreadSummary(ctx context.Context, req *dto.UnreadSummaryRequest) (model.UnreadSummary, error)
	AddMember(context.Context, *dto.AddMemberRequest) (uuid.UUID, error)
	RemoveMember(context.Context, *dto.RemoveMemberRequest) error
	Transfer(context.Context, *dto.TransferThreadRequest) (uuid.UUID, error)
	CompleteBotControl(context.Context, *dto.CompleteBotControlRequest) error
}

type ThreadVariablesOperator interface {
	Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error)
	Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error)
	Locate(ctx context.Context, threadID uuid.UUID) (*model.ThreadVariables, error)
	Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error)
}

type ThreadManagementServer struct {
	impb.UnimplementedThreadManagementServer

	inMapper              *mapper.ThreadInConverter
	outMapper             *mapper.ThreadOutConverter
	threadManager         ThreadManagementService
	threadVariables       ThreadVariablesOperator
	threadCreatorsFactory service.ThreadCreatorsFactoryProvider
}

func NewThreadService(threadManager ThreadManagementService, threadVariables ThreadVariablesOperator, threadCreatorsFactory service.ThreadCreatorsFactoryProvider) *ThreadManagementServer {
	return &ThreadManagementServer{
		threadManager:         threadManager,
		inMapper:              &mapper.ThreadInConverter{},
		outMapper:             &mapper.ThreadOutConverter{},
		threadVariables:       threadVariables,
		threadCreatorsFactory: threadCreatorsFactory,
	}
}

func (ts *ThreadManagementServer) Create(ctx context.Context, req *impb.ThreadManagementCreateRequest) (*impb.ThreadManagementCreateResponse, error) {
	initOptions := make([]func(*service.CreateThreadRequest), 0)

	switch r := req.GetType().(type) {
	case *impb.ThreadManagementCreateRequest_Direct:
		initOptions = append(initOptions, service.WithKind(model.ThreadDirect), service.WithDirectConfig(mapper.MapPeerFromProto(r.Direct.GetMember())))
	default:
		return nil, errors.InvalidArgument("received unknown create thread request type", errors.WithID("grpc.thread.create.unknown_type"), errors.WithValue("type", fmt.Sprintf("%T", r)))
	}

	createRequest := service.NewCreateThreadRequest(req.GetDomainId(), mapper.MapPeerFromProto(req.GetInitiator()), initOptions...)

	thread, err := ts.threadCreatorsFactory.Create(ctx, createRequest)
	if err != nil {
		return nil, err
	}

	pbThread := ts.outMapper.ConvertToThread(thread)

	return &impb.ThreadManagementCreateResponse{Thread: pbThread}, nil
}

func (ts *ThreadManagementServer) Get(ctx context.Context, req *impb.GetThreadRequest) (*impb.Thread, error) {
	getReq, err := ts.inMapper.ConvertGet(req)
	if err != nil {
		return nil, err
	}

	thread, err := ts.threadManager.Get(ctx, getReq)
	if err != nil {
		return nil, err
	}

	return ts.outMapper.ConvertToThread(thread), nil
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

	next, threads := utils.ProcessPagination(int(req.GetSize()), threads)

	res := impb.SearchThreadResponse{Next: next}

	for _, threadModel := range threads {
		res.Items = append(res.Items, ts.outMapper.ConvertToThread(threadModel))
	}

	return &res, nil
}

func (ts *ThreadManagementServer) GetUnreadSummary(ctx context.Context, req *impb.GetUnreadSummaryRequest) (*impb.GetUnreadSummaryResponse, error) {
	selfID, err := uuid.Parse(req.GetSelfId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid self id", errors.WithCause(err), errors.WithID("grpc.thread.unread_summary.self_id"))
	}

	summary, err := ts.threadManager.GetUnreadSummary(ctx, &dto.UnreadSummaryRequest{
		SelfID:   selfID,
		DomainID: req.GetDomainId(),
	})
	if err != nil {
		return nil, err
	}

	return &impb.GetUnreadSummaryResponse{
		UnreadChats:    summary.Chats,
		UnreadMessages: summary.Messages,
	}, nil
}

// Transfer implements [thread.ThreadManagementServer].
func (ts *ThreadManagementServer) Transfer(ctx context.Context, req *impb.TransferRequest) (*impb.TransferResponse, error) {
	internalRequest, err := ts.inMapper.ConvertTransferThreadRequest(req)
	if err != nil {
		return nil, err
	}

	newMember, err := ts.threadManager.Transfer(ctx, internalRequest)
	if err != nil {
		return nil, err
	}

	return &impb.TransferResponse{Member: &impb.ThreadMember{
		Id: newMember.String(),
	}}, nil
}

func (ts *ThreadManagementServer) SearchLeft(ctx context.Context, req *impb.SearchLeftRequest) (*impb.SearchLeftResponse, error) {
	search, err := ts.inMapper.ConvertSearchLeft(req)
	if err != nil {
		return nil, err
	}

	threads, err := ts.threadManager.SearchLeft(ctx, search)
	if err != nil {
		return nil, err
	}

	next, threads := utils.ProcessPagination(int(req.GetSize()), threads)

	res := &impb.SearchLeftResponse{Next: next}
	for _, t := range threads {
		res.Items = append(res.Items, ts.outMapper.ConvertToThread(t))
	}

	return res, nil
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

// CompleteBotControl is called by flow_manager when a bot schema finishes execution.
func (ts *ThreadManagementServer) CompleteBotControl(ctx context.Context, req *impb.CompleteBotControlRequest) (*impb.CompleteBotControlResponse, error) {
	tid, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id", errors.WithCause(err))
	}

	mid, err := uuid.Parse(req.GetMemberId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid member_id", errors.WithCause(err))
	}

	if err = ts.threadManager.CompleteBotControl(ctx, &dto.CompleteBotControlRequest{
		ThreadID: tid,
		MemberID: mid,
		DomainID: int(req.GetDomainId()),
	}); err != nil {
		return nil, err
	}

	return &impb.CompleteBotControlResponse{}, nil
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
	threadID, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread id format", errors.WithCause(err))
	}

	vars, err := ts.threadVariables.Locate(ctx, threadID)
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
