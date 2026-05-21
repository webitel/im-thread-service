package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type MessageHistoryService interface {
	Search(context.Context, *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error)
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
	hmiDTO := mapper.MapSearchMessageHistoryRequest2HistoryMessageInputDTO(req)

	messages, pageInfo, err := s.messageHistorySearcher.Search(ctx, hmiDTO)
	if err != nil {
		return nil, err
	}

	resp := mapper.MapMessage2SearchMessageHistoryResponse(messages)
	resp.From = mapper.GetUniqueFrom(messages)

	if pageInfo.HasNextPage {
		resp.NextCursor = &impb.HistoryMessageCursorResponse{
			Id: pageInfo.NextCursor.ID.String(),
		}
	}

	if pageInfo.HasPrevPage {
		resp.PrevCursor = &impb.HistoryMessageCursorResponse{
			Id: pageInfo.PrevCursor.ID.String(),
		}
	}

	return resp, nil
}
