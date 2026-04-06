package grpc

import (
	"context"
	"log/slog"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

var _ impb.MessageServer = (*MessageServer)(nil)

type MessageService interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
	SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error)
	SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error)
	Read(ctx context.Context, in *dto.ReadMessageRequest) error
}

type MessageServer struct {
	impb.UnimplementedMessageServer

	logger  *slog.Logger
	handler MessageService
}

// SendText implements threadv1.MessageServer.
func (m *MessageServer) SendText(ctx context.Context, in *impb.SendTextRequest) (*impb.SendTextResponse, error) {
	out, err := m.handler.SendText(ctx, mapper.MapToSendTextRequest(in))
	if err != nil {
		return nil, err
	}

	return mapper.MapToSendTextResponse(out), nil
}

// SendImage implements threadv1.MessageServer.
func (m *MessageServer) SendImage(ctx context.Context, in *impb.SendImageRequest) (*impb.SendImageResponse, error) {
	out, err := m.handler.SendImage(ctx, mapper.MapToSendImageRequest(in))
	if err != nil {
		m.logger.Error("failed to send image", "error", err)
		return nil, err
	}

	return mapper.MapToSendImageResponse(out), nil
}

// SendDocument implements threadv1.MessageServer.
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

func NewMessageService(logger *slog.Logger, handler MessageService) *MessageServer {
	return &MessageServer{
		logger:  logger,
		handler: handler,
	}
}
