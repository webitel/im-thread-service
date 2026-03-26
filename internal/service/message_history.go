package service

import (
	"context"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/im-thread-service/internal/utils"
)

var (
	_ MessageHistorySearcher = (*messageHistory)(nil)
)

type MessageHistorySearcher interface {
	Search(context.Context, *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error)
}

type messageHistory struct {
	messageHistoryStore store.MessageHistory
}

func NewMessageHistory(messageHistoryStore store.MessageHistory) *messageHistory {
	return &messageHistory{
		messageHistoryStore: messageHistoryStore,
	}
}

func (s *messageHistory) Search(ctx context.Context, hmiDTO *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error) {
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

	pageInfo, err := query.BuildPageInfo(&historyMessages, func(m *dto.HistoryMessage) (queryobject.MessageHistoryCursor, error) {
		return queryobject.MessageHistoryCursor{
			ID: m.ID,
		}, nil
	})

	if err != nil {
		return nil, queryobject.PageInfo[queryobject.MessageHistoryCursor]{}, err
	}

	messages := mapHistoryMessagesToMessage(historyMessages)

	return messages, pageInfo, nil
}

func mapHistoryMessagesToMessage(history []*dto.HistoryMessage) []*model.Message {
	return utils.Map(history, func(histMsg *dto.HistoryMessage) *model.Message {
		//? add identity scoped mapping [id]*object to reduce memory allocations?
		docs := mapDocuments(histMsg.Documents)
		images := mapImages(histMsg.Images)

		return &model.Message{
			ID:        histMsg.ID,
			ThreadID:  histMsg.ThreadID,
			From:      shared.Peer{ID: histMsg.SenderID},
			Body:      histMsg.Body,
			Type:      model.MessageType(histMsg.Type),
			CreatedAt: histMsg.CreatedAt,
			UpdatedAt: histMsg.UpdatedAt,
			Images:    images,
			Documents: docs,
			Metadata:  histMsg.Metadata,
		}
	})
}

func mapDocuments(historyDocs []*dto.HistoryDocument) []*model.MessageDocument {
	return utils.Map(historyDocs, func(doc *dto.HistoryDocument) *model.MessageDocument {
		return &model.MessageDocument{
			ID:        doc.ID,
			MessageID: doc.MessageID,
			FileID:    int64(doc.FileID),
			Name:      doc.Name,
			Mime:      doc.Mime,
			Size:      doc.Size,
			CreatedAt: doc.CreatedAt,
		}
	})
}

func mapImages(historyImages []*dto.HistoryImage) []*model.MessageImage {
	return utils.Map(historyImages, func(histImg *dto.HistoryImage) *model.MessageImage {
		return &model.MessageImage{
			ID:        histImg.ID,
			MessageID: histImg.MessageID,
			FileID:    int64(histImg.FileID),
			Mime:      histImg.Mime,
			Width:     int32(histImg.Width),
			Height:    int32(histImg.Height),
			CreatedAt: histImg.CreatedAt,
		}
	})
}
