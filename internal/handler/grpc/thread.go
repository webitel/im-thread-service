package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/api/thread/v1"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/utils"
)

type (
	ThreadService struct {
		impb.UnimplementedThreadManagementServer

		threadManager service.ThreadManager
	}
)

func NewThreadService(threadManager service.ThreadManager) *ThreadService {
	return &ThreadService{
		threadManager: threadManager,
	}
}

func (ts *ThreadService) Search(ctx context.Context, req *impb.ThreadSearchRequest) (*impb.SearchThreadResponse, error) {
	search := mapper.MapThreadSearchRequestToDTO(req)
	threads, err := ts.threadManager.Search(ctx, search)
	if err != nil {
		return nil, err
	}

	next, threads := utils.ProcessPagination(int(req.Size), threads)

	protoResponse := mapper.MapThreadsToProtoThreadList(threads)
	{
		protoResponse.Next = next
	}

	return protoResponse, nil
}
