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

// SearchThreadMessagesHistory searches for messages in the given thread based on the given request.
// The request can contain filters on message id, thread id, sender id, receiver id, type, and body.
// The response contains the messages found by the search, as well as the pagination information.
// The pagination information contains the next flag, which indicates whether there are more messages after the current page,
// and the paging map, which contains the message id and the corresponding pagination information for each message in the page.
// The from field in the response contains the unique sender ids of the messages in the page.
// The next field in the response is based on the direction of the search and the presence of the cursor in the request.
// If the request contains a cursor, the next field is set to true if there are more messages in the same direction as the cursor,
// and false otherwise. If the request does not contain a cursor, the next field is set to true if there are more messages in the same direction as the search.
// The search direction is determined by the direction field in the request. If the direction field is true, the search is done in the backward direction,
// and false otherwise. If the request does not contain a direction field, the search is done in the forward direction by default.
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
