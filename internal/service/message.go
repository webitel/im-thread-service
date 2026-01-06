package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	guards "github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
)

type Messager interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
}

type MessageService struct {
	uow      store.UnitOfWork
	logger   *slog.Logger
	threader ThreadManager
}

func NewMessageService(uow store.UnitOfWork, logger *slog.Logger, threader ThreadManager) *MessageService {
	return &MessageService{
		uow:      uow,
		logger:   logger,
		threader: threader,
	}
}

var _ Messager = (*MessageService)(nil)

func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure direct thread: %w", err)
	}

	msg := model.NewTextMessage(t.Id, in.From, in.To, in.Body)

	var resp *dto.SendTextResponse
	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		// TODO what we need here
		err = uow.Outbox().Publish(txCtx, "im.messages", events.MessageCreated{
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
