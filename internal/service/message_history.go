package service

import (
	"context"
	"slices"

	"github.com/google/uuid"
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

func (s *MessageHistoryService) SearchDialogsMessageHistory(ctx context.Context, req *dto.DialogsMessageHistoryInputDTO) (*dto.DialogsMessageHistoryOutputDTO, error) {
	fields := req.Fields
	if len(fields) > 0 && !slices.Contains(fields, "member") {
		fields = append(fields, "member")
	}

	query := queryobject.NewDialogsMessageHistoryQueryObject().
		WithFields(fields).
		WithDomainIDFilter(req.DomainID).
		WithThreadIDFilter(req.ThreadID).
		WithSenderIDsFilter(req.SenderIds...).
		WithTypesFilter(req.Types...).
		WithPeriodFilter(req.PeriodFrom, req.PeriodTo).
		WithLimit(req.Size).
		WithCursor(req.Cursor)

	messages, err := s.messageHistoryStore.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	pageInfo, err := query.BuildPageInfo(&messages, func(m *model.Message) (queryobject.MessageHistoryCursor, error) {
		return queryobject.MessageHistoryCursor{ID: m.ID}, nil
	})
	if err != nil {
		return nil, err
	}

	out := groupMessagesIntoSessions(messages)

	if pageInfo.HasNextPage {
		out.NextCursor = pageInfo.NextCursor.ID.String()
	}
	if pageInfo.HasPrevPage {
		out.PrevCursor = pageInfo.PrevCursor.ID.String()
	}

	return out, nil
}

func groupMessagesIntoSessions(messages []*model.Message) *dto.DialogsMessageHistoryOutputDTO {
	var (
		sessionsBy   = make(map[uuid.UUID]*dto.SessionMessageHistory)
		fromBy       = make(map[uuid.UUID]*model.ThreadDialog)
		sessionOrder = make([]uuid.UUID, 0)
	)

	for _, m := range messages {
		if m.Member == nil {
			continue
		}
		sid := m.Member.ID

		sess, ok := sessionsBy[sid]
		if !ok {
			var invitedBy uuid.UUID
			if m.Member.InvitedBy != nil {
				invitedBy = *m.Member.InvitedBy
			}
			var leaveReason string
			if m.Member.LeaveReason != nil {
				leaveReason = *m.Member.LeaveReason
			}

			sess = &dto.SessionMessageHistory{
				MemberID:    sid,
				InvitedBy:   invitedBy,
				ThreadRole:  m.Member.ThreadRole,
				LeaveReason: leaveReason,
			}
			sessionsBy[sid] = sess
			fromBy[sid] = m.Member
			sessionOrder = append(sessionOrder, sid)
		}

		sess.Messages = append(sess.Messages, m)
	}

	out := &dto.DialogsMessageHistoryOutputDTO{
		Items: make([]*dto.SessionMessageHistory, 0, len(sessionOrder)),
		From:  make([]*model.ThreadDialog, 0, len(sessionOrder)),
	}
	for _, sid := range sessionOrder {
		out.Items = append(out.Items, sessionsBy[sid])
		out.From = append(out.From, fromBy[sid])
	}

	return out
}
