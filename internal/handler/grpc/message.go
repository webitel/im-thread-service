package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

var _ impb.MessageServer = (*MessageServer)(nil)

type MessageService interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
	SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error)
	Read(ctx context.Context, in *dto.ReadMessageRequest) error
	SendLocation(ctx context.Context, msg *model.Message) (*model.Message, error)
	SendContact(ctx context.Context, msg *model.Message) (*model.Message, error)
	SendInteractive(ctx context.Context, msg *model.Message) (*model.Message, error)
	SendInteractiveCallback(ctx context.Context, callback *model.InteractiveCallback) (*model.InteractiveCallback, error)
	SendSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
	EditMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
}

type MessageServer struct {
	impb.UnimplementedMessageServer

	logger  *slog.Logger
	handler MessageService
}

func NewMessageService(logger *slog.Logger, handler MessageService) *MessageServer {
	return &MessageServer{
		logger:  logger,
		handler: handler,
	}
}

func (m *MessageServer) SendText(ctx context.Context, in *impb.SendTextRequest) (*impb.SendTextResponse, error) {
	out, err := m.handler.SendText(ctx, mapper.MapToSendTextRequest(in))
	if err != nil {
		return nil, err
	}

	return mapper.MapToSendTextResponse(out), nil
}

func (m *MessageServer) SendDocument(ctx context.Context, in *impb.SendDocumentRequest) (*impb.SendDocumentResponse, error) {
	out, err := m.handler.SendDocument(ctx, mapper.MapToSendDocumentRequest(in))
	if err != nil {
		m.logger.Error("failed to send document", "error", err)

		return nil, err
	}

	return mapper.MapToSendDocumentResponse(out), nil
}

func (m *MessageServer) Read(ctx context.Context, in *impb.ReadMessageRequest) (*impb.ReadMessageResponse, error) {
	err := m.handler.Read(ctx, mapper.MapToReadMessageRequest(in))
	if err != nil {
		m.logger.Error("failed to read message", "error", err)

		return nil, err
	}

	return &impb.ReadMessageResponse{}, nil
}

func (m *MessageServer) SendLocation(ctx context.Context, in *impb.SendLocationRequest) (*impb.SendMessageResponse, error) {
	sendAs, _ := uuid.Parse(in.GetSendAs())

	msg := &model.Message{
		DomainID:       in.GetDomainId(),
		IdempotencyKey: in.GetSendId(),
		From:           mapper.MapPeerFromProto(in.GetFrom()),
		SendTo:         mapper.MapPeerFromProto(in.GetTo()),
		Type:           model.MessageTypeLocation,
		Metadata:       in.GetMetadata().AsMap(),
		SenderID:       uuid.UUID{},
		Location: &model.MessageLocation{
			Address:   in.Address,
			Latitude:  in.GetLatitude(),
			Longitude: in.GetLongitude(),
			Name:      in.Name,
		},
		SendAs:  &sendAs,
		ReplyTo: model.NewReplyTarget(mapper.ParseOptionalUUID(in.GetReplyToMessageId())),
	}

	saved, err := m.handler.SendLocation(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{
			in.GetTo(),
		},
		Id: saved.ID.String(),
	}, nil
}

func (m *MessageServer) SendContact(ctx context.Context, in *impb.SendContactRequest) (*impb.SendMessageResponse, error) {
	sendAs, _ := uuid.Parse(in.GetSendAs())

	msg := &model.Message{
		IdempotencyKey: in.GetSendId(),
		DomainID:       in.GetDomainId(),
		From:           mapper.MapPeerFromProto(in.GetFrom()),
		SendTo:         mapper.MapPeerFromProto(in.GetTo()),
		Type:           model.MessageTypeContact,
		Metadata:       in.GetMetadata().AsMap(),
		Contact: &model.MessageContact{
			Name:        in.Name,
			PhoneNumber: in.PhoneNumber,
			Email:       in.Email,
		},
		SendAs:  &sendAs,
		ReplyTo: model.NewReplyTarget(mapper.ParseOptionalUUID(in.GetReplyToMessageId())),
	}

	saved, err := m.handler.SendContact(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{
			in.GetTo(),
		},
		Id: saved.ID.String(),
	}, nil
}

func (m *MessageServer) SendInteractive(ctx context.Context, in *impb.SendInteractiveMessageRequest) (*impb.SendMessageResponse, error) {
	interactive, err := mapper.ConvertInteractivePbToDomain(in.GetInteractive())
	if err != nil {
		return nil, err
	}

	sendAs, _ := uuid.Parse(in.GetSendAs())

	msg := &model.Message{
		IdempotencyKey: in.GetSendId(),
		DomainID:       in.GetDomainId(),
		From:           mapper.MapPeerFromProto(in.GetFrom()),
		SendTo:         mapper.MapPeerFromProto(in.GetTo()),
		Body:           in.GetBody(),
		Type:           model.MessageTypeInteractive,
		Metadata:       in.GetMetadata().AsMap(),
		Interactive:    interactive,
		Images:         mapper.ConvertPbImagesToDomain(in.GetInteractive().GetImages()),
		Documents:      mapper.ConvertPbDocumentsToDomain(in.GetInteractive().GetDocuments()),
		SendAs:         &sendAs,
	}

	processed, err := m.handler.SendInteractive(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{
			in.GetTo(),
		},
		Id: processed.ID.String(),
	}, nil
}

func (m *MessageServer) SendInteractiveCallback(ctx context.Context, in *impb.InteractiveCallbackRequest) (*impb.InteractiveCallbackResponse, error) {
	from := mapper.MapPeerFromProto(in.GetReactedBy())

	callback := &model.InteractiveCallback{
		ReactedBy:    from.ID,
		InReplyTo:    uuid.MustParse(in.GetInReplyTo()),
		ButtonCode:   in.GetButtonCode(),
		CallbackData: in.GetCallbackData(),
	}

	processed, err := m.handler.SendInteractiveCallback(ctx, callback)
	if err != nil {
		return nil, err
	}

	return &impb.InteractiveCallbackResponse{
		ReactedBy:    mapper.MapPeerToProto(shared.Peer{ID: processed.ReactedBy, Type: from.Type}),
		InReplyTo:    processed.InReplyTo.String(),
		ButtonCode:   processed.ButtonCode,
		CallbackData: processed.CallbackData,
		ReactedAt:    processed.ReactedAtUnix(),
	}, nil
}

func (m *MessageServer) SendSystemMessage(ctx context.Context, in *impb.SendSystemMessageRequest) (*impb.SendMessageResponse, error) {
	saved, err := m.handler.SendSystemMessage(ctx, mapper.ConvertPbSystemMessageToDomain(in))
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		To: []*impb.Peer{
			in.GetTo(),
		},
		Id: saved.ID.String(),
	}, nil
}

func (m *MessageServer) EditMessage(ctx context.Context, in *impb.EditMessageRequest) (*impb.EditMessageResponse, error) {
	edited, err := m.handler.EditMessage(ctx, mapper.ConvertPbEditMessageToDomain(in))
	if err != nil {
		m.logger.Error("failed to edit message", "error", err)

		return nil, err
	}

	return mapper.MapToEditMessageResponse(edited), nil
}
