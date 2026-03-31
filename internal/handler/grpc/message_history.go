package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
)

type MessageHistoryService interface {
	Search(context.Context, *dto.HistoryMessageInputDTO) (model.MessageSlice, error)
}

type (
	MessageHistoryServer struct {
		impb.UnimplementedMessageHistoryServer

		messageHistorySearcher MessageHistoryService
	}
)

func NewMessageHistoryServer(messageHistorySearcher MessageHistoryService) *MessageHistoryServer {
	return &MessageHistoryServer{
		messageHistorySearcher: messageHistorySearcher,
	}
}

func (s *MessageHistoryServer) SearchThreadMessagesHistory(ctx context.Context, req *impb.SearchMessageHistoryRequest) (*impb.SearchMessageHistoryResponse, error) {
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

	var isBackward bool
	hadCursor := req.Cursor != nil

	if hadCursor {
		isBackward = !req.Cursor.Direction
	} else {
		isBackward = true
	}

	paging := messages.GetPaging(next, isBackward, hadCursor)
	{
		resp = mapper.MapMessage2SearchMessageHistoryResponse(messages)
		resp.From = mapper.GetUniqueFrom(messages)
		resp.Next = next
		resp.Paging = mapper.MapPaging(paging)
	}

	return resp, nil
}
