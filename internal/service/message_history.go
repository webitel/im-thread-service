package service

import (
	"context"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type MessageHistoryStore interface {
	Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Message, error)
}

type MessageHistoryService struct {
	messageHistoryStore MessageHistoryStore
}

func NewMessageHistory(messageHistoryStore store.MessageHistory) *MessageHistoryService {
	return &MessageHistoryService{
		messageHistoryStore: messageHistoryStore,
	}
}

func (s *MessageHistoryService) Search(ctx context.Context, hmiDTO *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error) {
	query := queryobject.NewMessageHistoryQuery().
		WithFields(hmiDTO.Fields).
		WithCursor(hmiDTO.Cursor).
		WithDomainIDsFilter(hmiDTO.DomainID).
		WithIDsFilter(hmiDTO.IDs...).
		WithSenderIDsFilter(hmiDTO.SenderIDs...).
		WithThreadIDsFilter(hmiDTO.ThreadIDs...).
		WithLimit(hmiDTO.Size).
		WithTypeFilter(hmiDTO.Types...)

	historyMessages, err := s.messageHistoryStore.Search(ctx, query)
	if err != nil {
		return nil, queryobject.PageInfo[queryobject.MessageHistoryCursor]{}, err
	}

	pageInfo, err := query.BuildPageInfo(&historyMessages, func(m *model.Message) (queryobject.MessageHistoryCursor, error) {
		return queryobject.MessageHistoryCursor{
			ID: m.ID,
		}, nil
	})
	if err != nil {
		return nil, queryobject.PageInfo[queryobject.MessageHistoryCursor]{}, err
	}

	return historyMessages, pageInfo, nil
}
