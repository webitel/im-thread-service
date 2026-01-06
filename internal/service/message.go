package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	guards "github.com/webitel/im-thread-service/internal/service/guard"
	"github.com/webitel/im-thread-service/internal/store"
)

type Messager interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
}

type MessageService struct {
	store  store.Store
	logger *slog.Logger
}

func NewMessageService(store store.Store, logger *slog.Logger) *MessageService {
	return &MessageService{store: store, logger: logger}
}

var _ Messager = (*MessageService)(nil)

func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	// TODO CanSend

	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	msg := model.NewTextMessage(uuid.New(), in.From, in.To, in.Body)

	var resp *dto.SendTextResponse
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		saved, err := s.store.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		// TODO what we need here
		err = s.store.Outbox().Publish(txCtx, "im.messages", events.MessageCreated{
			MessageID:  saved.Id,
			ThreadID:   saved.ThreadId,
			From:       saved.From,
			To:         saved.To,
			Body:       saved.Text,
			Type:       saved.Type,
			OccurredAt: saved.CreatedAt,
		})
		if err != nil {
			return err
		}

		resp = &dto.SendTextResponse{Id: saved.Id, To: saved.To}
		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_text_failed", "err", err)
		return nil, err
	}

	return resp, nil
}
