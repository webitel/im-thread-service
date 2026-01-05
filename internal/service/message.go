package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

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

func NewMessageService(store store.Store, logger *slog.Logger) *MessageService {
	return &MessageService{store: store, logger: logger}
}

var _ Messager = (*MessageService)(nil)

func (s *MessageService) SendText(
	ctx context.Context,
	in *dto.SendTextRequest,
) (*dto.SendTextResponse, error) {
	// Prepare and validate the request
	if err := s.prepareAndValidate(in); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	var resp *dto.SendTextResponse

	// Execute within a single transaction to ensure Atomicity (Message + Outbox Event)
	err := s.store.WithTx(ctx, func(txCtx context.Context) error {
		msg := &model.Message{
			From: model.Peer{Id: in.From.Id},
			To:   model.Peer{Id: in.To.Id},
			Text: in.Body,
			Type: model.MessageTypeText,
		}

		// 1. Persist the message to the database
		saved, err := s.store.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}

		// 2. Prepare the domain event
		event := events.MessageCreated{
			MessageID:  saved.Id,
			ThreadID:   saved.ThreadId,
			From:       saved.From,
			To:         saved.To,
			Body:       saved.Text,
			Type:       saved.Type,
			OccurredAt: time.Now().UTC(),
		}

		// 3. Save event to the Outbox table (Transactional Outbox Pattern)
		err = s.store.Outbox().Publish(txCtx, "im.messages", event)
		if err != nil {
			return fmt.Errorf("failed to publish to outbox: %w", err)
		}

		resp = &dto.SendTextResponse{
			Id: saved.Id,
			To: saved.To,
		}

		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "transactional message sending failed",
			slog.Any("error", err),
			slog.String("from_id", in.From.Id.String()),
			slog.String("to_id", in.To.Id.String()),
		)
		return nil, err
	}

	return resp, nil
}

// prepareAndValidate handles data sanitization, normalization, and business rules
func (s *MessageService) prepareAndValidate(req *dto.SendTextRequest) error {
	// 1. Basic empty check
	if req.Body == "" {
		return errors.New("message body cannot be empty")
	}

	// 2. Check for valid UTF-8 sequence to prevent DB encoding errors
	// PostgreSQL throws a fatal error if it receives invalid byte sequences for UTF-8
	if !utf8.ValidString(req.Body) {
		// Option A: Return error
		// return errors.New("message body contains invalid UTF-8 characters")

		// Option B: Sanitize (remove invalid bytes)
		req.Body = strings.ToValidUTF8(req.Body, "")
	}

	// 3. Unicode Normalization (NFC)
	// Ensures consistent storage and search. For example, combined characters
	// like 'й' will be represented by a single code point regardless of the sender's OS (iOS/Android/Web)
	req.Body = norm.NFC.String(req.Body)

	// 4. Identity validation
	if req.From.Id == uuid.Nil {
		return errors.New("sender (From) peer ID is required")
	}
	if req.To.Id == uuid.Nil {
		return errors.New("recipient (To) peer ID is required")
	}

	return nil
}
