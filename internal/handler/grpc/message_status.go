package grpc

import (
	"context"
	"log/slog"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
)

var _ impb.MessageStatusServer = (*MessageStatusServer)(nil)

// MessageStatusReporter applies per-recipient delivery confirmations and
// returns the number of statuses actually changed.
type MessageStatusReporter interface {
	MarkDelivered(ctx context.Context, receipts []*model.StatusReceipt) (int64, error)
	MarkRead(ctx context.Context, receipts []*model.ReadReceipt) (int64, error)
	MarkFailed(ctx context.Context, receipts []*model.StatusReceipt) (int64, error)
}

// MessageStatusServer is an internal service-to-service API: im-delivery
// reports stream ACKs and push deliveries, im-providers reports provider
// webhook receipts (delivered/read/failed).
type MessageStatusServer struct {
	impb.UnimplementedMessageStatusServer

	logger  *slog.Logger
	handler MessageStatusReporter
}

func NewMessageStatusServer(logger *slog.Logger, handler MessageStatusReporter) *MessageStatusServer {
	return &MessageStatusServer{
		logger:  logger,
		handler: handler,
	}
}

func (s *MessageStatusServer) MarkDelivered(ctx context.Context, in *impb.MarkDeliveredRequest) (*impb.MarkStatusResponse, error) {
	receipts, err := mapper.MapToDeliveryReceipts(in.GetReceipts())
	if err != nil {
		return nil, err
	}

	updated, err := s.handler.MarkDelivered(ctx, receipts)
	if err != nil {
		s.logger.Error("failed to mark delivered", "error", err)

		return nil, err
	}

	return &impb.MarkStatusResponse{Updated: updated}, nil
}

func (s *MessageStatusServer) MarkRead(ctx context.Context, in *impb.MarkReadRequest) (*impb.MarkStatusResponse, error) {
	receipts, err := mapper.MapToReadReceipts(in.GetReceipts())
	if err != nil {
		return nil, err
	}

	updated, err := s.handler.MarkRead(ctx, receipts)
	if err != nil {
		s.logger.Error("failed to mark read", "error", err)

		return nil, err
	}

	return &impb.MarkStatusResponse{Updated: updated}, nil
}

func (s *MessageStatusServer) MarkFailed(ctx context.Context, in *impb.MarkFailedRequest) (*impb.MarkStatusResponse, error) {
	receipts, err := mapper.MapToFailureReceipts(in.GetReceipts())
	if err != nil {
		return nil, err
	}

	updated, err := s.handler.MarkFailed(ctx, receipts)
	if err != nil {
		s.logger.Error("failed to mark failed", "error", err)

		return nil, err
	}

	return &impb.MarkStatusResponse{Updated: updated}, nil
}
