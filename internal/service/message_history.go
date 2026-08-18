package service

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type MessageHistoryStore interface {
	Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Message, error)
}

type MessageRevisionStore interface {
	Search(ctx context.Context, messageID uuid.UUID, domainID int32, callerID uuid.UUID) ([]*model.MessageChangeEntry, error)
}

type MessageHistoryService struct {
	messageHistoryStore  MessageHistoryStore
	messageRevisionStore MessageRevisionStore
}

func NewMessageHistory(messageHistoryStore store.MessageHistory, messageRevisionStore store.MessageRevisionStore) *MessageHistoryService {
	return &MessageHistoryService{
		messageHistoryStore:  messageHistoryStore,
		messageRevisionStore: messageRevisionStore,
	}
}

func (s *MessageHistoryService) GetRevisions(ctx context.Context, in *dto.GetMessageRevisionsRequest) ([]*model.MessageChangeEntry, error) {
	if in == nil || in.MessageID == uuid.Nil {
		return nil, errors.InvalidArgument("message id is required", errors.WithID("service.message_history.get_revisions"))
	}

	if in.CallerID == uuid.Nil {
		return nil, errors.InvalidArgument("caller identity is required", errors.WithID("service.message_history.get_revisions"))
	}

	entries, err := s.messageRevisionStore.Search(ctx, in.MessageID, in.DomainID, in.CallerID)
	if err != nil {
		if errors.Is(err, store.ErrMessageNotVisible) {
			return nil, errors.Forbidden(
				"message history is not available: the message does not exist or the caller is not a member of its thread",
				errors.WithID("service.message_history.get_revisions.not_allowed"),
			)
		}

		return nil, err
	}

	return entries, nil
}

func (s *MessageHistoryService) Search(ctx context.Context, hmiDTO *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error) {
	query := queryobject.NewMessageHistoryQuery().
		WithFields(hmiDTO.Fields).
		WithCursor(hmiDTO.Cursor).
		WithDomainIDsFilter(hmiDTO.DomainID).
		WithIDsFilter(hmiDTO.IDs...).
		WithSenderIDsFilter(hmiDTO.SenderIDs...).
		WithThreadIDsFilter(hmiDTO.ThreadIDs...).
		WithCallerLimitation(hmiDTO.CallerID, hmiDTO.ThreadIDs).
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

func (s *MessageHistoryService) SearchMessages(ctx context.Context, req *dto.SearchMessagesInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error) {
	var empty queryobject.PageInfo[queryobject.MessageHistoryCursor]

	if req == nil {
		return nil, empty, errors.InvalidArgument(
			"search term is required",
			errors.WithID("service.message_history.search_messages"),
		)
	}

	term := strings.TrimSpace(req.Term)
	if term == "" || utf8.RuneCountInString(term) > 256 {
		return nil, empty, errors.InvalidArgument("search term is required", errors.WithID("service.message_history.search_messages"))
	}

	if req.CallerID == uuid.Nil {
		return nil, empty, errors.InvalidArgument("caller identity is required", errors.WithID("service.message_history.search_messages"))
	}

	query := queryobject.NewMessageSearchQuery().
		WithFields(req.Fields).
		WithTermFilter(term).
		WithDomainIDFilter(req.DomainID).
		WithThreadIDsFilter(req.ThreadIDs...).
		WithSenderIDsFilter(req.SenderIDs...).
		WithTypeFilter(req.Types...).
		WithCallerScope(req.CallerID).
		WithLimit(req.Size).
		WithCursor(req.Cursor)

	messages, err := s.messageHistoryStore.Search(ctx, query)
	if err != nil {
		return nil, empty, err
	}

	pageInfo, err := query.BuildPageInfo(&messages, func(m *model.Message) (queryobject.MessageHistoryCursor, error) {
		return queryobject.MessageHistoryCursor{
			ID: m.ID,
		}, nil
	})
	if err != nil {
		return nil, empty, err
	}

	return messages, pageInfo, nil
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
		WithSenderIDsFilter(req.SenderIDs...).
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
