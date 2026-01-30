package mapper

import (
	"time"

	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
	"github.com/webitel/im-thread-service/internal/utils/set"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func MapSearchMessageHistoryRequest2HistoryMessageInputDTO(mhr *impb.SearchMessageHistoryRequest) *dto.HistoryMessageInputDTO {
	var (
		ids         = utils.Map(mhr.Ids, utils.IdsParser)
		threadIds   = utils.Map(mhr.ThreadIds, utils.IdsParser)
		senderIds   = utils.Map(mhr.SenderIds, utils.IdsParser)
		receiverIds = utils.Map(mhr.ReceiverIds, utils.IdsParser)
		types       = utils.Map(mhr.Types, func(i int32) int { return int(i) })
		cursor      *dto.HistoryMessageCursor
	)

	if mhr.Cursor != nil {
		cursor = new(dto.HistoryMessageCursor)
		{
			id, _ := uuid.Parse(mhr.Cursor.Id)
			cursor.Id = id
			cursor.CreatedAt = time.UnixMicro(mhr.Cursor.CreatedAt)
			cursor.Direction = mhr.Cursor.Direction
		}
	}

	return &dto.HistoryMessageInputDTO{
		Fields:      mhr.Fields,
		Ids:         ids,
		ThreadIds:   threadIds,
		SenderIds:   senderIds,
		ReceiverIds: receiverIds,
		Size:        int(mhr.GetSize()),
		Types:       types,
		Cursor:      cursor,
		DomainID:    int(mhr.GetDomainId()),
	}
}

func MapMessage2SearchMessageHistoryResponse(messages []*model.Message) *impb.SearchMessageHistoryResponse {
	responseMessages := utils.Map(messages, func(m *model.Message) *impb.HistoryMessage {
		var (
			docs   = mapDocs(m.Documents)
			images = mapImages(m.Images)
		)

		md, err := toAnyMap(m.Metadata)
		if err != nil {
			return nil
		}

		return &impb.HistoryMessage{
			Id:         m.ID.String(),
			ThreadId:   m.ThreadID.String(),
			SenderId:   m.From.ID.String(),
			ReceiverId: m.To.ID.String(),
			Body:       m.Text,
			Type:       int32(m.Type),
			Metadata:   md,
			CreatedAt:  max(m.CreatedAt.UnixMilli(), 0),
			UpdatedAt:  max(m.UpdatedAt.UnixMilli(), 0),
			Documents:  docs,
			Images:     images,
		}
	})

	return &impb.SearchMessageHistoryResponse{
		Messages: responseMessages,
		Next:     true,
	}
}

func GetUniqueFrom(messages []*model.Message) []string {
	var (
		set = set.New[uuid.UUID](0)
	)

	for _, message := range messages {
		set.Insert(message.From.ID)
	}

	return utils.Map(set.Slice(), func(p uuid.UUID) string { return p.String() })
}

func MapPaging(paging *model.Paging) *impb.Paging {
	if paging == nil {
		return nil
	}

	var (
		after, before *impb.HistoryMessageCursor
	)

	if paging.After != nil {
		after = newPBHistoryCursor(paging.After.Id, paging.After.CreatedAt, paging.After.Direction)
	}

	if paging.Before != nil {
		before = newPBHistoryCursor(paging.Before.Id, paging.Before.CreatedAt, paging.Before.Direction)
	}

	return &impb.Paging{
		Cursors: &impb.Cursors{
			After:  after,
			Before: before,
		},
	}
}

func newPBHistoryCursor(id uuid.UUID, createdAt time.Time, direction bool) *impb.HistoryMessageCursor {
	return &impb.HistoryMessageCursor{
		CreatedAt: createdAt.UnixMicro(),
		Id:        id.String(),
		Direction: direction,
	}
}

func mapDocs(docs []*model.MessageDocument) []*impb.Document {
	return utils.Map(docs, func(md *model.MessageDocument) *impb.Document {
		return &impb.Document{
			Id:        md.ID.String(),
			MessageId: md.MessageID.String(),
			FileId:    md.FileID,
			Name:      md.Name,
			Mime:      md.Mime,
			Size:      md.Size,
			CreatedAt: md.CreatedAt.UnixMilli(),
			Url:       md.URL,
		}
	})
}

func mapImages(images []*model.MessageImage) []*impb.Image {
	return utils.Map(images, func(mi *model.MessageImage) *impb.Image {
		return &impb.Image{
			Id:        mi.ID.String(),
			MessageId: mi.MessageID.String(),
			FileId:    mi.FileID,
			Mime:      mi.Mime,
			Width:     mi.Width,
			Height:    mi.Height,
			CreatedAt: mi.CreatedAt.UnixMilli(),
			Url:       mi.URL,
		}
	})
}

func toAnyMap(src map[string]any) (map[string]*anypb.Any, error) {
	dst := make(map[string]*anypb.Any, len(src))

	for k, v := range src {
		spb, _ := structpb.NewValue(v)
		a, err := anypb.New(spb)
		if err != nil {
			return nil, err
		}
		dst[k] = a
	}

	return dst, nil
}
