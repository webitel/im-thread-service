package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
)

var (
	_ impb.MessageServer = (*MessageService)(nil)
)

type MessageService struct {
	impb.UnimplementedMessageServer

	logger  *slog.Logger
	handler service.Messager
}

func NewMessageService(logger *slog.Logger, handler service.Messager) *MessageService {
	return &MessageService{
		logger:  logger,
		handler: handler,
	}
}

func (m *MessageService) SendText(ctx context.Context, in *impb.SendTextRequest) (*impb.SendTextResponse, error) {
	out, err := m.handler.SendText(ctx, mapper.MapToSendTextRequest(in))
	if err != nil {
		return nil, err
	}

	return mapper.MapToSendTextResponse(out), nil
}

func (m *MessageService) SendImage(ctx context.Context, in *impb.SendImageRequest) (*impb.SendImageResponse, error) {
	out, err := m.handler.SendImage(ctx, mapper.MapToSendImageRequest(in))
	if err != nil {
		m.logger.Error("failed to send image", "error", err)
		return nil, err
	}

	return mapper.MapToSendImageResponse(out), nil
}

func (m *MessageService) SendDocument(ctx context.Context, in *impb.SendDocumentRequest) (*impb.SendDocumentResponse, error) {
	out, err := m.handler.SendDocument(ctx, mapper.MapToSendDocumentRequest(in))
	if err != nil {
		m.logger.Error("failed to send document", "error", err)
		return nil, err
	}

	return mapper.MapToSendDocumentResponse(out), nil
}

func (m *MessageService) Read(ctx context.Context, in *impb.ReadMessageRequest) (*impb.ReadMessageResponse, error) {
	err := m.handler.Read(ctx, mapper.MapToReadMessageRequest(in))
	if err != nil {
		m.logger.Error("failed to read message", "error", err)
		return nil, err
	}

	return &impb.ReadMessageResponse{}, nil
}

func (m *MessageService) SendInteractive(ctx context.Context, in *impb.SendInteractiveMessageRequest) (*impb.SendMessageResponse, error) {
	interactive, err := mapper.MapInteractive(in.GetInteractive())
	if err != nil {
		m.logger.Error("error mapping interactive message", "error", err)
		return nil, err
	}

	message := &model.Message{
		DomainID:    in.GetDomainId(),
		From:        mapper.MapPeerFromProto(in.GetFrom()),
		SendTo:      mapper.MapPeerFromProto(in.GetTo()),
		Text:        in.GetInteractive().GetBody(),
		Type:        model.MessageTypeInteractive,
		Metadata:    in.GetMetadata().AsMap(),
		Interactive: interactive,
	}

	saved, err := m.handler.SendInteractive(ctx, message)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{mapper.MapPeerToProto(saved.SendTo)},
		Id: saved.ID.String(),
	}, nil
}

func (m *MessageService) SendLocation(ctx context.Context, r *impb.SendLocationRequest) (*impb.SendMessageResponse, error) {
	msg := &model.Message{
		DomainID: r.GetDomainId(),
		From:     mapper.MapPeerFromProto(r.GetFrom()),
		SendTo:   mapper.MapPeerFromProto(r.GetTo()),
		Type:     model.MessageTypeLocation,
		Metadata: r.GetMetadata().AsMap(),
		Location: &model.Location{
			Address:   r.Address,
			Latitude:  r.Latitude,
			Longitude: r.Longitude,
			Name:      r.Name,
		},
	}

	result, err := m.handler.SendLocation(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{mapper.MapPeerToProto(result.SendTo)},
		Id: result.ID.String(),
	}, nil
}

func (m *MessageService) SendContact(ctx context.Context, r *impb.SendContactRequest) (*impb.SendMessageResponse, error) {
	msg := &model.Message{
		DomainID: r.GetDomainId(),
		From:     mapper.MapPeerFromProto(r.GetFrom()),
		SendTo:   mapper.MapPeerFromProto(r.To),
		Type:     model.MessageTypeContact,
		Metadata: r.GetMetadata().AsMap(),
		Contact: &model.Contact{
			Name:        r.Name,
			Email:       r.Email,
			PhoneNumber: r.PhoneNumber,
			Metadata:    r.GetContactMetadata(),
		},
	}

	saved, err := m.handler.SendContact(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{mapper.MapPeerToProto(saved.SendTo)},
		Id: saved.ID.String(),
	}, nil
}

func (m *MessageService) SendInteractionCallback(ctx context.Context, in *impb.InteractionCallback) (*impb.InteractionCallback, error) {
	var (
		messageID, _ = uuid.Parse(in.GetInReplyTo())
		clickedBy = mapper.MapPeerFromProto(in.GetClickedBy())
	)
	
	var callback = &model.ButtonsCallback{
		MessageID:    messageID,
		ButtonCode:   in.GetButtonCode(),
		CallbackData: in.GetCallbackData(),
		ClickedBy:    clickedBy.ID,
	}

	saved, err := m.handler.HandleInteractiveCallback(ctx, callback)
	if err != nil {
		return nil, err
	}

	return &impb.InteractionCallback{
		InReplyTo:    saved.MessageID.String(),
		ButtonCode:   saved.ButtonCode,
		CallbackData: saved.CallbackData,
		ClickedAt:    saved.GetClickedAtUnix(),
		ClickedBy:    mapper.MapPeerToProto(clickedBy),
	}, nil
}