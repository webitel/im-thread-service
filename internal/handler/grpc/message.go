package grpc

import (
	"context"
	"log/slog"

	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

var _ impb.MessageServer = &MessageService{}

type MessageService struct {
	impb.UnimplementedMessageServer

	logger  *slog.Logger
	handler service.Messager
}

// SendText implements threadv1.MessageServer.
func (m *MessageService) SendText(ctx context.Context, in *impb.SendTextRequest) (*impb.SendTextResponse, error) {
	out, err := m.handler.SendText(ctx, dto.NewSendTextRequest(in))
	if err != nil {
		return nil, err
	}

	return &impb.SendTextResponse{
		Id: out.Id.String(),
		To: mapper.ToProtoPeer(out.To),
	}, nil
}

// SendDocument implements threadv1.MessageServer.
func (m *MessageService) SendDocument(context.Context, *impb.SendDocumentRequest) (*impb.SendDocumentResponse, error) {
	panic("unimplemented")
}

// SendImage implements threadv1.MessageServer.
func (m *MessageService) SendImage(context.Context, *impb.SendImageRequest) (*impb.SendImageResponse, error) {
	panic("unimplemented")
}

// mustEmbedUnimplementedMessageServer implements threadv1.MessageServer.
func (m *MessageService) mustEmbedUnimplementedMessageServer() {
	panic("unimplemented")
}

func NewMessageService(logger *slog.Logger, handler service.Messager) *MessageService {
	return &MessageService{
		logger:  logger,
		handler: handler,
	}
}
