package grpc

import (
	"context"
	"log/slog"

	impb "github.com/webitel/im-thread-service/gen/go/api/thread/v1"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
)

var _ impb.MessageServer = &MessageService{}

type MessageService struct {
	impb.UnimplementedMessageServer

	logger  *slog.Logger
	handler service.Messager
}

// SendText implements threadv1.MessageServer.
func (m *MessageService) SendText(ctx context.Context, in *impb.SendTextRequest) (*impb.SendTextResponse, error) {
	out, err := m.handler.SendText(ctx, mapper.MapToSendTextRequest(in))
	if err != nil {
		return nil, err
	}

	return mapper.MapToSendTextResponse(out), nil
}

// SendImage implements threadv1.MessageServer.
func (m *MessageService) SendImage(ctx context.Context, in *impb.SendImageRequest) (*impb.SendImageResponse, error) {
	out, err := m.handler.SendImage(ctx, mapper.MapToSendImageRequest(in))
	if err != nil {
		m.logger.Error("failed to send image", "error", err)
		return nil, err
	}

	return mapper.MapToSendImageResponse(out), nil
}

// SendDocument implements threadv1.MessageServer.
func (m *MessageService) SendDocument(ctx context.Context, in *impb.SendDocumentRequest) (*impb.SendDocumentResponse, error) {
	out, err := m.handler.SendDocument(ctx, mapper.MapToSendDocumentRequest(in))
	if err != nil {
		m.logger.Error("failed to send document", "error", err)
		return nil, err
	}

	return mapper.MapToSendDocumentResponse(out), nil
}

func NewMessageService(logger *slog.Logger, handler service.Messager) *MessageService {
	return &MessageService{
		logger:  logger,
		handler: handler,
	}
}
