package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

type Messager interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
}

type MessageService struct {
	store  store.Store
	logger *slog.Logger
}

func NewMessageService(store store.Store, logger *slog.Logger) Messager {
	return &MessageService{store: store, logger: logger}
}

var _ Messager = (*MessageService)(nil)

func (s *MessageService) SendText(
	ctx context.Context,
	in *dto.SendTextRequest,
) (*dto.SendTextResponse, error) {
	if err := s.validateSendText(in); err != nil {
		return nil, err
	}

	var resp *dto.SendTextResponse

	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		msg := &model.Message{
			// TODO mocked for now
			ThreadId: uuid.Nil,
			From:     model.Peer{Id: in.From.Id},
			To:       model.Peer{Id: in.To.Id},
			Text:     in.Body,
			Type:     model.MessageTypeText,
		}

		saved, err := s.store.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		event := events.MessageCreated{
			MessageID:  saved.Id,
			ThreadID:   saved.ThreadId,
			From:       saved.From,
			To:         saved.To,
			Body:       saved.Text,
			Type:       saved.Type,
			OccurredAt: saved.CreatedAt,
		}

		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}

		err = s.store.Outbox().Add(txCtx, store.OutboxRecord{
			ID:      uuid.New(),
			Topic:   "im.messages",
			Payload: payload,
			Metadata: map[string]string{
				"event_type": event.EventType(),
			},
		})
		if err != nil {
			return err
		}

		resp = &dto.SendTextResponse{
			Id: saved.Id,
			To: saved.To,
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("send text failed: %w", err)
	}

	return resp, nil
}

func (m *MessageService) validateSendText(req *dto.SendTextRequest) error {
	if req.Body == "" {
		return errors.New("message body is empty")
	}
	if req.From.Id == uuid.Nil || req.To.Id == uuid.Nil {
		return errors.New("invalid peer id")
	}
	return nil
}
