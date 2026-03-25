package grpc

import (
	"context"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
)

type MessageHistoryService struct {
	impb.UnimplementedMessageHistoryServer

	messageHistorySearcher service.MessageHistorySearcher
}

func NewMessageHistoryService(messageHistorySearcher service.MessageHistorySearcher) *MessageHistoryService {
	return &MessageHistoryService{
		messageHistorySearcher: messageHistorySearcher,
	}
}
func (s *MessageHistoryService) SearchThreadMessagesHistory(ctx context.Context, req *impb.SearchMessageHistoryRequest) (*impb.SearchMessageHistoryResponse, error) {
	hmiDTO := mapper.MapSearchMessageHistoryRequest2HistoryMessageInputDTO(req)

	messages, pageInfo, err := s.messageHistorySearcher.Search(ctx, hmiDTO)
	if err != nil {
		return nil, err
	}

	resp := mapper.MapMessage2SearchMessageHistoryResponse(messages)
	resp.From = mapper.GetUniqueFrom(messages)

	if pageInfo.HasNextPage {
		resp.NextCursor = &impb.HistoryMessageCursor{
			Id: pageInfo.NextCursor.ID.String(),
		}
	}

	if pageInfo.HasPrevPage {
		resp.PrevCursor = &impb.HistoryMessageCursor{
			Id: pageInfo.PrevCursor.ID.String(),
		}
	}

	return resp, nil
}
