package mapper

import (
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
	"github.com/webitel/im-thread-service/internal/utils/set"
)

func MapSearchMessageHistoryRequest2HistoryMessageInputDTO(mhr *impb.SearchMessageHistoryRequest) *dto.HistoryMessageInputDTO {
	var (
		ids       = utils.Map(mhr.GetIds(), utils.IDsParser)
		threadIDs = utils.Map([]string{mhr.GetThreadId()}, utils.IDsParser)
		senderIDs = utils.Map(mhr.GetSenderIds(), utils.IDsParser)
		types     = utils.Map(mhr.GetTypes(), func(i int32) int { return int(i) })
		cursor    *dto.HistoryMessageCursor
	)

	if mhr.GetCursor() != nil {
		cursor = new(dto.HistoryMessageCursor)
		{
			id, _ := uuid.Parse(mhr.GetCursor().GetId())
			cursor.ID = id
			cursor.Direction = mhr.GetCursor().GetBefore()
		}
	}

	return &dto.HistoryMessageInputDTO{
		Fields:    mhr.GetFields(),
		IDs:       ids,
		ThreadIDs: threadIDs,
		SenderIDs: senderIDs,
		Size:      int(mhr.GetSize()),
		Types:     types,
		Cursor:    cursor,
		DomainID:  int(mhr.GetDomainId()),
	}
}

func MapMessage2SearchMessageHistoryResponse(messages []*model.Message) *impb.SearchMessageHistoryResponse {
	responseMessages := utils.Map(messages, func(m *model.Message) *impb.HistoryMessage {
		var (
			docs   = mapDocs(m.Documents)
			images = mapImages(m.Images)
		)

		md, err := structpb.NewStruct(m.Metadata)
		if err != nil {
			return nil
		}

		return &impb.HistoryMessage{
			Id:          m.ID.String(),
			ThreadId:    m.ThreadID.String(),
			SenderId:    m.SenderID.String(),
			Body:        m.Body,
			Type:        int32(m.Type),
			Metadata:    md,
			CreatedAt:   max(m.CreatedAt.UnixMilli(), 0),
			UpdatedAt:   max(m.UpdatedAt.UnixMilli(), 0),
			Documents:   docs,
			Images:      images,
			Location:    mapLocation(m.Location),
			Contact:     mapContact(m.Contact),
			System:      mapSystem(m.System),
			Interactive: mapInteractive(m.Interactive),
		}
	})

	return &impb.SearchMessageHistoryResponse{
		Items: responseMessages,
	}
}

func mapLocation(location *model.MessageLocation) *impb.Location {
	if location == nil {
		return nil
	}

	return &impb.Location{
		Latitude:  location.Latitude,
		Longitude: location.Longitude,
		Name:      location.Name,
		Address:   location.Address,
	}
}

func mapInteractive(interactive *model.MessageInteractive) *impb.Interactive {
	if interactive == nil {
		return nil
	}

	protoInteractive := new(impb.Interactive)
	{
		protoInteractive.SingleUse = interactive.SingleUse
	}

	if interactive.Kind.ListReply != nil {
		protoInteractive.Kind = &impb.Interactive_ListReply{
			ListReply: mapInteractiveList(interactive.Kind.ListReply),
		}
	} else if interactive.Kind.Markup != nil {
		protoInteractive.Kind = &impb.Interactive_Markup{
			Markup: mapInteractiveMarkup(interactive.Kind.Markup),
		}
	}

	return protoInteractive
}

func mapInteractiveMarkup(interactiveMarkup *model.KeyboardButtonMarkup) *impb.KeyboardMarkup {
	if interactiveMarkup == nil {
		return nil
	}

	pbRows := make([]*impb.KeyboardRow, 0, len(interactiveMarkup.Rows))
	for _, row := range interactiveMarkup.Rows {
		if row == nil {
			continue
		}

		pbButtons := make([]*impb.KeyboardButton, 0, len(row.Buttons))
		for _, btn := range row.Buttons {
			if mappedBtn := mapButton(btn); mappedBtn != nil {
				pbButtons = append(pbButtons, mappedBtn)
			}
		}

		if len(pbButtons) > 0 {
			pbRows = append(pbRows, &impb.KeyboardRow{
				Buttons: pbButtons,
			})
		}
	}

	return &impb.KeyboardMarkup{
		Rows: pbRows,
	}
}

func mapInteractiveList(interactiveList *model.KeyboardListReply) *impb.KeyboardListReply {
	if interactiveList == nil {
		return nil
	}

	sections := make([]*impb.KeyboardRowWithSection, 0, len(interactiveList.Sections))
	for _, section := range interactiveList.Sections {
		mappedSection := mapListReplySection(section)
		if mappedSection != nil {
			sections = append(sections, mappedSection)
		}
	}

	return &impb.KeyboardListReply{
		MainButtonTitle: interactiveList.Title,
		Sections:        sections,
	}
}

func mapListReplySection(section *model.ListReplySection) *impb.KeyboardRowWithSection {
	if section == nil {
		return nil
	}

	buttons := make([]*impb.KeyboardButton, 0, len(section.Buttons))
	for _, btn := range section.Buttons {
		mappedBtn := mapButton(btn)
		if mappedBtn != nil {
			buttons = append(buttons, mappedBtn)
		}
	}

	return &impb.KeyboardRowWithSection{
		Section: section.Section,
		Buttons: buttons,
	}
}

func mapButton(button *model.KeyboardButton) *impb.KeyboardButton {
	if button == nil {
		return nil
	}

	buttonMetadata, _ := structpb.NewStruct(button.Metadata)

	pbButton := new(impb.KeyboardButton)
	{
		pbButton.Id = button.ID
		pbButton.Label = button.Label
		pbButton.Metadata = buttonMetadata
	}

	switch button.Type {
	case model.ActionTypeCallback:
		if button.Data != nil {
			pbButton.Kind = &impb.KeyboardButton_Callback{
				Callback: &impb.KeyboardButtonCallback{
					Data: *button.Data,
				},
			}
		}
	case model.ActionTypeURL:
		if button.URL != nil {
			pbButton.Kind = &impb.KeyboardButton_Url{
				Url: &impb.KeyboardButtonURL{
					Url: *button.URL,
				},
			}
		}
	case model.ActionTypeRequest:
		if button.Action != nil {
			pbButton.Kind = &impb.KeyboardButton_Request{
				Request: &impb.KeyboardButtonRequest{
					Action: *button.Action,
				},
			}
		}
	}

	return pbButton
}

func mapSystem(system *model.MessageSystem) *impb.System {
	if system == nil {
		return nil
	}

	pbMd, _ := structpb.NewStruct(system.Metadata)

	return &impb.System{
		Type:      system.Type,
		Metadata:  pbMd,
		MessageId: system.MessageID,
	}
}

func mapContact(contact *model.MessageContact) *impb.Contact {
	if contact == nil {
		return nil
	}

	return &impb.Contact{
		Name:        contact.Name,
		Email:       contact.Email,
		PhoneNumber: contact.PhoneNumber,
	}
}

func GetUniqueFrom(messages []*model.Message) []*impb.ThreadMember {
	set := set.New[model.ThreadDialog](0)

	for _, message := range messages {
		if mem := message.Member; mem != nil {
			set.Insert(*mem)
		}
	}

	threadConverter := new(ThreadOutConverter)

	return utils.Map(set.Slice(), func(p model.ThreadDialog) *impb.ThreadMember {
		role := threadConverter.ConvertThreadRole(p.ThreadRole)

		return &impb.ThreadMember{
			Id:        p.ID.String(),
			ContactId: p.ContactID.String(),
			Role:      role,
		}
	})
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
