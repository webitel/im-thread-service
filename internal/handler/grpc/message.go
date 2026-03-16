package grpc

import (
	"context"
	"log/slog"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/chain"
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

func (m *MessageService) SendInteractive(ctx context.Context, in *impb.SendInteractiveButtonsRequest) (*impb.SendMessageResponse, error) {
	msg := mapper.InteractiveRequestToMessage(in)
	
	handler := chain.Process(
		m.handler.SendInteractiveMessage,
		chain.ValidationWrapper,
		chain.EnsureThreadWrapper[*model.Message, *model.Message](m.handler.Threader().EnsureDirectThread),
	)

	msg, err := handler(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &impb.SendMessageResponse{
		Id: msg.ID.String(),
		To: []*impb.Peer{in.To},
	}, nil
}

func (m *MessageService) SendInteractiveCallback(ctx context.Context, r *impb.SendInteractiveCallbackRequest) (*impb.SendInteractiveCallbackResponse, error) {
	interactionModel := mapper.InteractiveCallbackRequestToInteraction(r)

	handler := chain.Process(
		m.handler.HandleInteraction,
		chain.ValidationWrapper,
	)

	resultCallback, err := handler(ctx, interactionModel)
	if err != nil {
		return nil, err
	}

	return mapper.InteractionToCallbackResponse(resultCallback), nil
}