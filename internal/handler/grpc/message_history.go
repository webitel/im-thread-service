package grpc

import (
	"context"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type MessageHistoryService interface {
	Search(context.Context, *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error)
	SearchMessages(ctx context.Context, req *dto.SearchMessagesInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error)
	SearchLeftThreads(ctx context.Context, req *dto.LeftThreadsMessageHistoryInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error)
	GetRevisions(ctx context.Context, req *dto.GetMessageRevisionsRequest) ([]*model.MessageChangeEntry, error)
}

// ThreadHeads reads a thread's current update_seq, handed out with history.
type ThreadHeads interface {
	Head(ctx context.Context, threadID string) (int64, error)
}

type (
	MessageHistoryServer struct {
		impb.UnimplementedMessageHistoryServer

		messageHistorySearcher MessageHistoryService
		updates                ThreadHeads
	}
)

func NewMessageHistoryServer(messageHistorySearcher MessageHistoryService, updates ThreadHeads) *MessageHistoryServer {
	return &MessageHistoryServer{
		messageHistorySearcher: messageHistorySearcher,
		updates:                updates,
	}
}

func (s *MessageHistoryServer) SearchThreadMessagesHistory(ctx context.Context, req *impb.SearchMessageHistoryRequest) (*impb.SearchMessageHistoryResponse, error) {
	hmiDTO := mapper.MapSearchMessageHistoryRequest2HistoryMessageInputDTO(req)

	// Read the head before the page: a change in between is replayed later, never skipped.
	var head int64

	if threadID := req.GetThreadId(); threadID != "" && s.updates != nil {
		var err error
		if head, err = s.updates.Head(ctx, threadID); err != nil {
			return nil, err
		}
	}

	messages, pageInfo, err := s.messageHistorySearcher.Search(ctx, hmiDTO)
	if err != nil {
		return nil, err
	}

	resp := mapper.MapMessage2SearchMessageHistoryResponse(messages, hmiDTO.CallerID)
	resp.From = mapper.GetUniqueFrom(messages)
	resp.LastUpdateSeq = head

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

func (s *MessageHistoryServer) SearchMessages(ctx context.Context, req *impb.SearchMessagesRequest) (*impb.SearchMessageHistoryResponse, error) {
	searchDTO := mapper.MapSearchMessagesRequest2SearchMessagesInputDTO(req)

	messages, pageInfo, err := s.messageHistorySearcher.SearchMessages(ctx, searchDTO)
	if err != nil {
		return nil, err
	}

	resp := mapper.MapMessage2SearchMessageHistoryResponse(messages, searchDTO.CallerID)
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

func (s *MessageHistoryServer) GetMessageRevisions(ctx context.Context, req *impb.GetMessageRevisionsRequest) (*impb.GetMessageRevisionsResponse, error) {
	revisions, err := s.messageHistorySearcher.GetRevisions(ctx, mapper.MapGetMessageRevisionsRequest2DTO(req))
	if err != nil {
		return nil, err
	}

	return mapper.MapRevisions2GetMessageRevisionsResponse(revisions), nil
}

func (s *MessageHistoryServer) SearchLeftThreadsMessageHistory(ctx context.Context, req *impb.SearchLeftThreadsMessageHistoryRequest) (*impb.SearchMessageHistoryResponse, error) {
	requestDTO := mapper.MapSearchLeftThreadsMessageHistoryRequest2LeftThreadsMessageHistoryInputDTO(req)

	messages, pageInfo, err := s.messageHistorySearcher.SearchLeftThreads(ctx, requestDTO)
	if err != nil {
		return nil, err
	}

	// Left-threads history has no caller context; reacted_by_me stays false.
	resp := mapper.MapMessage2SearchMessageHistoryResponse(messages, uuid.Nil)
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
