package mapper

import (
	"strconv"

	baseerrs "errors"

	"github.com/webitel/im-thread-service/gen/go/thread/v1"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

func MapInteractive(pb *thread.Interactive) (*model.Interactive, error) {
	if pb == nil {
		return nil, errors.InvalidArgument("interactive payload is required")
	}

	out := &model.Interactive{
		Body:      pb.GetBody(),
		Footer:    pb.Footer,
		SingleUse: pb.GetSingleUse(),
	}

	header, err := mapInteractiveHeader(pb.Header)
	if err != nil {
		return nil, err
	}
	out.Header = header

	kind, err := mapInteractiveKind(pb.GetKind())
	if err != nil {
		return nil, err
	}

	switch v := kind.(type) {
	case *model.KeyboardButton:
		out.CTA = v
	case *model.KeyboardListReply:
		out.ListReply = v
	case *model.KeyboardMarkup:
		out.Markup = v
	}

	return out, nil
}

func mapInteractiveHeader(pb any) (*model.InteractiveHeader, error) {
	switch t := pb.(type) {
	case *thread.Interactive_Document:
		docs, err := mapDocuments(t.Document.GetDocuments())
		if err != nil {
			return nil, err
		}
		return &model.InteractiveHeader{Documents: docs}, nil

	case *thread.Interactive_Image:
		images, err := mapImagesFromInput(t.Image.GetImages())
		if err != nil {
			return nil, err
		}
		return &model.InteractiveHeader{Images: images}, nil

	case *thread.Interactive_Text:
		return &model.InteractiveHeader{Text: &t.Text}, nil

	default:
		return nil, nil
	}
}

func mapDocuments(pbs []*impb.DocumentInput) ([]*model.MessageDocument, error) {
	out := make([]*model.MessageDocument, 0, len(pbs))
	for _, d := range pbs {
		id, err := parseOptionalID(d.Id)
		if err != nil {
			return nil, err
		}
		out = append(out, &model.MessageDocument{
			FileID: int64(id),
			Name:   d.GetFileName(),
			Mime:   d.GetMimeType(),
			Size:   d.GetSizeBytes(),
			URL:    d.GetUrl(),
		})
	}
	return out, nil
}

func mapImagesFromInput(pbs []*thread.ImageInput) ([]*model.MessageImage, error) {
	out := make([]*model.MessageImage, 0, len(pbs))
	for _, i := range pbs {
		id, err := parseOptionalID(i.Id)
		if err != nil {
			return nil, err
		}
		out = append(out, &model.MessageImage{
			FileID: int64(id),
			Name:   i.GetName(),
			Mime:   i.GetMimeType(),
			URL:    i.Link,
		})
	}
	return out, nil
}

func mapInteractiveKind(pb any) (any, error) {
	switch k := pb.(type) {
	case *thread.Interactive_Cta:
		btn, err := mapButton(k.Cta)
		if err != nil {
			return nil, err
		}
		return btn, nil

	case *thread.Interactive_ListReply:
		reply, err := mapListReply(k.ListReply)
		if err != nil {
			return nil, err
		}
		return reply, nil

	case *thread.Interactive_Markup:
		return mapMarkupProto(k.Markup)

	default:
		return nil, errors.InvalidArgument("unexpected interactive type")
	}
}

func mapButton(pb *impb.KeyboardButton) (*model.KeyboardButton, error) {
	if pb == nil {
		return nil, nil
	}

	btn := &model.KeyboardButton{
		ButtonID:        pb.ButtonId,
		ButtonLabelText: pb.ButtonLabelText,
		Metadata: pb.GetMetadata().AsMap(),
	}

	switch k := pb.GetKind().(type) {
	case *thread.KeyboardButton_Callback:
		btn.Callback = &model.KeyboardButtonCallback{Data: k.Callback.GetData()}
	case *thread.KeyboardButton_Request:
		btn.Request = &model.KeyboardButtonRequest{Action: mapButtonAction(k.Request.GetAction())}
	case *thread.KeyboardButton_Url:
		btn.URL = &model.KeyboardButtonURL{URL: k.Url.GetUrl()}
	default:
		return nil, errors.InvalidArgument("unknown button kind")
	}

	return btn, nil
}

func mapListReply(pb *thread.KeyboardListReply) (*model.KeyboardListReply, error) {
	if pb == nil {
		return nil, nil
	}

	out := &model.KeyboardListReply{
		MainButtonTitle: pb.GetMainButtonTitle(),
		Sections:        make([]*model.KeyboardRowWithSections, len(pb.GetSections())),
	}

	for i, s := range pb.GetSections() {
		section, err := mapListSection(s)
		if err != nil {
			return nil, err
		}
		out.Sections[i] = section
	}

	return out, nil
}

func mapListSection(pb *thread.KeyboardRowWithSection) (*model.KeyboardRowWithSections, error) {
	out := &model.KeyboardRowWithSections{
		Section: pb.GetSection(),
		Buttons: make([]*model.KeyboardButton, 0, len(pb.GetButtons())),
	}

	for _, b := range pb.GetButtons() {
		btn, err := mapButton(b)
		if err != nil {
			return nil, err
		}
		out.Buttons = append(out.Buttons, btn)
	}

	return out, nil
}

func mapMarkupProto(pb *impb.KeyboardMarkup) (*model.KeyboardMarkup, error) {
	if pb == nil {
		return nil, nil
	}

	var errs []error
	matrix := make([][]*model.KeyboardButton, 0, len(pb.Rows))
	for _, row := range pb.Rows {
		btns := make([]*model.KeyboardButton, 0, len(row.Buttons))
		for _, b := range row.Buttons {
			btn, err := mapButton(b)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			btns = append(btns, btn)
		}
		matrix = append(matrix, btns)
	}

	return &model.KeyboardMarkup{ButtonsMatrix: matrix}, baseerrs.Join(errs...)
}

func mapButtonAction(a impb.KeyboardButtonRequestAction) model.KeyboardButtonRequestAction {
	switch a {
	case thread.KeyboardButtonRequestAction_CONTACT:
		return model.ContactButtonRequestAction
	case thread.KeyboardButtonRequestAction_LOCATION:
		return model.LocationButtonRequestAction
	default:
		return model.UnknownButtonRequestAction
	}
}

func parseOptionalID(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}