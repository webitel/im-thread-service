package service

import (
	"context"
	"slices"

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
		WithIdsFilter(hmiDTO.Ids...).
		WithSenderIdsFilter(hmiDTO.SenderIds...).
		WithThreadIdsFilter(hmiDTO.ThreadIds...).
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

func (s *MessageHistoryService) SearchLeftThreads(ctx context.Context, req *dto.LeftThreadsMessageHistoryInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error) {
	fields := req.Fields
	if len(fields) > 0 && !slices.Contains(fields, "member") {
		fields = append(fields, "member")
	}

	query := queryobject.NewLeftThreadsMessageHistoryQueryObject().
		WithFields(fields).
		WithDomainIDFilter(req.DomainID).
		WithThreadIDFilter(req.ThreadID).
		WithSenderIDsFilter(req.SenderIds...).
		WithTypesFilter(req.Types...).
		WithPeriodFilter(req.PeriodFrom, req.PeriodTo).
		WithLimit(req.Size).
		WithCursor(req.Cursor)

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
