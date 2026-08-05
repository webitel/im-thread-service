package mapper

import (
	"fmt"
	"strconv"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func convertInteractivePbButtonKindToDomain(in *impb.KeyboardButton) *model.KeyboardButton {
	switch v := in.GetKind().(type) {
	case *impb.KeyboardButton_Callback:
		return &model.KeyboardButton{
			ID:       in.GetId(),
			Label:    in.GetLabel(),
			Type:     model.ActionTypeCallback,
			Metadata: in.GetMetadata().AsMap(),
			Data:     &v.Callback.Data,
		}
	case *impb.KeyboardButton_Request:
		return &model.KeyboardButton{
			ID:       in.GetId(),
			Label:    in.GetLabel(),
			Type:     model.ActionTypeRequest,
			Metadata: in.GetMetadata().AsMap(),
			Action:   &v.Request.Action,
		}
	case *impb.KeyboardButton_Url:
		return &model.KeyboardButton{
			ID:       in.GetId(),
			Label:    in.GetLabel(),
			Type:     model.ActionTypeURL,
			Metadata: in.GetMetadata().AsMap(),
			URL:      &v.Url.Url,
		}
	default:
		return &model.KeyboardButton{
			ID:       in.GetId(),
			Label:    in.GetLabel(),
			Type:     model.ActionTypeNone,
			Metadata: in.GetMetadata().AsMap(),
		}
	}
}

func convertPbMarkupToDomain(markup *impb.KeyboardMarkup) (*model.KeyboardButtonMarkup, error) {
	rows := make([]*model.KeyboardButtonRow, 0, len(markup.GetRows()))
	uniqueCodes := make(map[string]struct{})

	for _, r := range markup.GetRows() {
		row := convertPbButtonRowToDomain(r)
		for _, b := range row.Buttons {
			if _, ok := uniqueCodes[b.ID]; ok {
				return nil, errors.InvalidArgument(
					"button id must be unique for each button per row of a keyboard markup",
					errors.WithCause(fmt.Errorf("duplicate button id: %s", b.ID)),
					errors.WithID("mapper.message.convertInteractivePbListReplyToDomain"),
				)
			}

			uniqueCodes[b.ID] = struct{}{}
		}

		rows = append(rows, row)
	}

	return &model.KeyboardButtonMarkup{
		Rows: rows,
	}, nil
}

func convertPbButtonRowToDomain(row *impb.KeyboardRow) *model.KeyboardButtonRow {
	buttons := make([]*model.KeyboardButton, 0, len(row.GetButtons()))
	for _, b := range row.GetButtons() {
		buttons = append(buttons, convertInteractivePbButtonKindToDomain(b))
	}

	return &model.KeyboardButtonRow{
		Buttons: buttons,
	}
}

func ConvertInteractivePbToDomain(in *impb.Interactive) (*model.MessageInteractive, error) {
	interactive := &model.MessageInteractive{
		SingleUse:       in.GetSingleUse(),
		Placement:       convertMenuPlacementPbToDomain(in.GetPlacement()),
		InputFieldState: convertInputFieldStatePbToDomain(in.GetInputFieldState()),
	}

	switch in.GetAttachments().(type) {
	case *impb.Interactive_Documents:
		interactive.Attachments = &model.MessageContent{Documents: true}
	case *impb.Interactive_Images:
		interactive.Attachments = &model.MessageContent{Images: true}
	}

	switch v := in.GetKind().(type) {
	case *impb.Interactive_ListReply:
		listReply, err := convertInteractivePbListReplyToDomain(v.ListReply)
		if err != nil {
			return nil, err
		}

		interactive.Kind = model.InteractiveKind{
			ListReply: listReply,
		}
	case *impb.Interactive_Markup:
		markup, err := convertPbMarkupToDomain(v.Markup)
		if err != nil {
			return nil, err
		}

		interactive.Kind = model.InteractiveKind{
			Markup: markup,
		}
	}

	return interactive, nil
}

func convertMenuPlacementPbToDomain(in impb.MenuPlacement) model.MenuPlacement {
	switch in {
	case impb.MenuPlacement_MENU_PLACEMENT_INLINE:
		return model.MenuPlacementInline
	case impb.MenuPlacement_MENU_PLACEMENT_PERSISTENT:
		return model.MenuPlacementPersistent
	default:
		return model.MenuPlacementUnspecified
	}
}

func convertInputFieldStatePbToDomain(in impb.InputFieldState) model.InputFieldState {
	switch in {
	case impb.InputFieldState_INPUT_FIELD_STATE_REGULAR:
		return model.InputFieldStateRegular
	case impb.InputFieldState_INPUT_FIELD_STATE_MINIMIZED:
		return model.InputFieldStateMinimized
	case impb.InputFieldState_INPUT_FIELD_STATE_HIDDEN:
		return model.InputFieldStateHidden
	default:
		return model.InputFieldStateUnspecified
	}
}

func convertInteractivePbListReplyToDomain(in *impb.KeyboardListReply) (*model.KeyboardListReply, error) {
	sections := make([]*model.ListReplySection, 0, len(in.GetSections()))

	uniqueCodes := make(map[string]struct{})
	for _, s := range in.GetSections() {
		buttons := make([]*model.KeyboardButton, 0, len(s.GetButtons()))
		for _, b := range s.GetButtons() {
			if _, exists := uniqueCodes[b.GetId()]; exists {
				return nil, errors.InvalidArgument(
					"button id must be unique for each button per list reply",
					errors.WithCause(fmt.Errorf("duplicate button id: %s", b.GetId())),
					errors.WithID("mapper.message.convertInteractivePbListReplyToDomain"),
				)
			}

			uniqueCodes[b.GetId()] = struct{}{}
			buttons = append(buttons, convertInteractivePbButtonKindToDomain(b))
		}

		sections = append(sections, &model.ListReplySection{
			Section: s.GetSection(),
			Buttons: buttons,
		})
	}

	return &model.KeyboardListReply{
		Title:    in.GetMainButtonTitle(),
		Sections: sections,
	}, nil
}

func ConvertPbDocumentsToDomain(in *impb.Documents) []*model.MessageDocument {
	if in == nil || len(in.GetDocuments()) == 0 {
		return nil
	}

	documents := make([]*model.MessageDocument, 0, len(in.GetDocuments()))
	for _, d := range in.GetDocuments() {
		var internalID int

		if d.GetId() != "" {
			var err error
			if internalID, err = strconv.Atoi(d.GetId()); err != nil {
				internalID = 0
			}
		}

		documents = append(documents, &model.MessageDocument{
			FileID: int64(internalID),
			Name:   d.GetFileName(),
			Mime:   d.GetMimeType(),
			Size:   d.GetSizeBytes(),
			URL:    d.GetUrl(),
		})
	}

	return documents
}

func ConvertPbImagesToDomain(in *impb.Images) []*model.MessageImage {
	if in == nil || len(in.GetImages()) == 0 {
		return nil
	}

	images := make([]*model.MessageImage, 0, len(in.GetImages()))
	for _, img := range in.GetImages() {
		var internalID int

		if img.GetId() != "" {
			var err error
			if internalID, err = strconv.Atoi(img.GetId()); err != nil {
				internalID = 0
			}
		}

		images = append(images, &model.MessageImage{
			FileID: int64(internalID),
			URL:    img.GetLink(),
			Name:   img.GetName(),
			Mime:   img.GetMimeType(),
		})
	}

	return images
}
