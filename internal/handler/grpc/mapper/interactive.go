package mapper

import (
	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func InteractiveRequestToMessage(in *impb.SendInteractiveButtonsRequest) *model.Message {
	return &model.Message{
		DomainID:       int32(in.GetDomainId()),
		Text:           in.GetTitle(),
		Type:           model.MessageTypeInteractive,
		Metadata:       make(map[string]any),
		From:           MapPeerFromProto(in.GetFrom()),
		SendTo:         MapPeerFromProto(in.GetTo()),
		MessageButtons: mapButtonRows(in.GetButtons()),
	}
}

func InteractiveCallbackRequestToInteraction(r *impb.SendInteractiveCallbackRequest) *model.MessageButtonInteraction {
	return &model.MessageButtonInteraction{
		InReplyTo:  uuid.MustParse(r.GetInReplyTo()),
		DomainID:   int(r.GetDomainId()),
		Action:     mapActionType(r.GetAction()),
		ButtonCode: r.GetButtonCode(),
		PressedBy:  uuid.MustParse(r.GetPressedBy()),
		Result:     mapInteractionResult(r.GetInteractionResult()),
	}
}

func InteractionToCallbackResponse(mbi *model.MessageButtonInteraction) *impb.SendInteractiveCallbackResponse {
	resPb := &impb.SendInteractiveCallbackResponse{
		Id:         mbi.ID.String(),
		InReplyTo:  mbi.InReplyTo.String(),
		Action:     string(mbi.Action),
		ButtonCode: mbi.ButtonCode,
		PressedBy:  mbi.PressedBy.String(),
		PressedAt:  mbi.GetPressedAt(),
	}

	switch res := mbi.Result.(type) {
	case *model.InteractionContact:
		resPb.InteractionResult = &impb.SendInteractiveCallbackResponse_ContactResult{
			ContactResult: &impb.ContactResult{
				InteractionId: res.InteractionID.String(),
				Name:          res.Name,
				PhoneNumber:   res.PhoneNumber,
				Metadata:      res.Metadata,
			},
		}
	case *model.InteractionPostback:
		resPb.InteractionResult = &impb.SendInteractiveCallbackResponse_PostbackResult{
			PostbackResult: &impb.PostbackResult{
				InteractionId: res.InteractionID.String(),
				CallbackData:  res.CallbackData,
			},
		}
	case *model.InteractionLocation:
		resPb.InteractionResult = &impb.SendInteractiveCallbackResponse_LocationResult{
			LocationResult: &impb.LocationResult{
				InteractionId: res.InteractionID.String(),
				Latitude:      res.Latitude,
				Longitude:     res.Longitude,
				City:          res.City,
				State:         res.State,
				Country:       res.Country,
				PostalCode:    res.PostalCode,
			},
		}
	}

	return resPb
}

func mapButtonRows(protoRows []*impb.ButtonRow) model.MessageButtonsMatrix {
	if protoRows == nil {
		return nil
	}

	matrix := make(model.MessageButtonsMatrix, 0, len(protoRows))
	for _, pr := range protoRows {
		row := make(model.MessageButtonRow, 0, len(pr.GetButtons()))
		for _, pb := range pr.GetButtons() {
			row = append(row, &model.MessageButtons{
				Code:         pb.GetCode(),
				Action:       mapActionType(pb.GetButtonActionType()),
				Title:        pb.GetTitle(),
				CallbackData: pb.GetCallbackData(),
				URL:          pb.GetUrl(),
			})
		}
		matrix = append(matrix, row)
	}
	return matrix
}

func mapActionType(protoType impb.ButtonActionType) model.ButtonActionType {
	switch protoType {
	case impb.ButtonActionType_CONTACT:
		return model.ContactAction
	case impb.ButtonActionType_LOCATION:
		return model.LocationAction
	case impb.ButtonActionType_POSTBACK:
		return model.PostbackAction
	case impb.ButtonActionType_REPLY:
		return model.ReplyAction
	case impb.ButtonActionType_URL:
		return model.URLAction
	default:
		return "unknown"
	}
}

func mapInteractionResult(r any) model.InteractionResult {
	switch res := r.(type) {
	case *impb.SendInteractiveCallbackRequest_ContactResult:
		return &model.InteractionContact{
			Name:        res.ContactResult.GetName(),
			PhoneNumber: res.ContactResult.GetPhoneNumber(),
			Metadata:    res.ContactResult.GetMetadata(),
		}
	case *impb.SendInteractiveCallbackRequest_PostbackResult:
		return &model.InteractionPostback{
			CallbackData: res.PostbackResult.GetCallbackData(),
		}
	case *impb.SendInteractiveCallbackRequest_LocationResult:
		return &model.InteractionLocation{
			Latitude:   res.LocationResult.Latitude,
			Longitude:  res.LocationResult.Longitude,
			City:       res.LocationResult.City,
			State:      res.LocationResult.State,
			Country:    res.LocationResult.Country,
			PostalCode: res.LocationResult.PostalCode,
		}
	default:
		return nil
	}
}