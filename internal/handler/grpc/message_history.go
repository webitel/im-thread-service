package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/api/thread/v1"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/utils"
)

type (
	MessageHistoryService struct {
		impb.UnimplementedMessageHistoryServer

		messageHistorySearcher service.MessageHistorySearcher
	}
)

func NewMessageHistoryService(messageHistorySearcher service.MessageHistorySearcher) *MessageHistoryService {
	return &MessageHistoryService{
		messageHistorySearcher: messageHistorySearcher,
	}
}

func (s *MessageHistoryService) SearchThreadMessagesHistory(ctx context.Context, req *impb.SearchMessageHistoryRequest) (*impb.SearchMessageHistoryResponse, error) {
	var (
		resp   *impb.SearchMessageHistoryResponse
		err    error
		hmiDTO = mapper.MapSearchMessageHistoryRequest2HistoryMessageInputDTO(req)
		next   bool
	)

	messages, err := s.messageHistorySearcher.Search(ctx, hmiDTO)
	if err != nil {
		return nil, err
	}

	next, messages = utils.ProcessPagination(int(req.Size), messages)

	resp = mapper.MapMessage2SearchMessageHistoryResponse(messages)
	resp.From = mapper.GetUniqueFrom(messages)
	resp.Next = next

	return resp, nil
}
