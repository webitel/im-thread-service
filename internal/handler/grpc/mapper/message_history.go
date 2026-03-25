package mapper

import (
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
		threadIds   = utils.Map([]string{mhr.ThreadId}, utils.IdsParser)
		senderIds   = utils.Map(mhr.SenderIds, utils.IdsParser)
		types       = utils.Map(mhr.Types, func(i int32) int { return int(i) })
		cursor      *dto.HistoryMessageCursor
	)

	if mhr.Cursor != nil {
		cursor = new(dto.HistoryMessageCursor)
		{
			id, _ := uuid.Parse(mhr.Cursor.Id)
			cursor.Id = id
			cursor.Direction = mhr.GetCursor().GetBefore()
		}
	}

	return &dto.HistoryMessageInputDTO{
		Fields:      mhr.Fields,
		Ids:         ids,
		ThreadIds:   threadIds,
		SenderIds:   senderIds,
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
		Items: responseMessages,
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
