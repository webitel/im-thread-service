package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/api/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	guards "github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
)

type Messager interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
	SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error)
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

// SendText handles normalization and multi-recipient distribution of text messages.
func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	// [VALIDATE] Ensure payload integrity
	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// // [THREAD] Resolve or initialize communication channel
	// t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
	// 	PeerFrom: &in.From,
	// 	PeerTo:   &in.To,
	// })
	// if err != nil {
	// 	return nil, err
	// }

	// [MODEL] Construct domain entity with pre-staged distribution events
	msg := model.NewTextMessage(uuid.New(), in.From, []model.Peer{in.To}, in.Body)

	var resp *dto.SendTextResponse

	// [ATOMIC] Execute persistence and outbox staging in a single transaction
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		// [DISPATCH] Propagate staged domain events to the Outbox store
		if err := s.dispatchEvents(txCtx, uow, msg); err != nil {
			return err
		}
		resp = &dto.SendTextResponse{ID: saved.ID, To: in.To}

		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_text_failed", "err", err)
		return nil, err
	}

	return resp, nil
}

// SendImage handles media attachments and transactional event propagation.
func (s *MessageService) SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error) {
	// [VALIDATE] Check media constraints and peer data
	if err := guards.SendImageGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// [THREAD]
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
	})
	if err != nil {
		return nil, err
	}

	// [MODEL] Initialize rich media message with mapped inputs
	msg := model.NewImageMessage(
		t.ID,
		in.From,
		[]model.Peer{in.To},
		in.Image.Body,
		s.mapImageInputs(in.Image.Images),
	)

	var resp *dto.SendImageResponse

	// [ATOMIC]
	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		// [DISPATCH]
		if err := s.dispatchEvents(txCtx, uow, msg); err != nil {
			return err
		}

		resp = &dto.SendImageResponse{ID: saved.ID, To: in.To}
		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_image_failed", "err", err)
		return nil, err
	}

	return resp, nil
}

// --- Internal Helpers ---

// dispatchEvents handles the propagation of staged domain events to the persistent Outbox.
func (s *MessageService) dispatchEvents(ctx context.Context, uow store.UnitOfWork, msg *model.Message) error {
	// [EXTRACT] Retrieve staged events from the domain entity.
	// Note: The model clears its internal queue upon this call to prevent double-dispatch.
	evs := msg.Events()

	// [INVARIANT_CHECK] Ensure consistency between database state and event stream.
	// If a message is persisted without its corresponding events, the system
	// enters an inconsistent state, thus we trigger a transaction rollback.
	if len(evs) == 0 {
		return fmt.Errorf("domain events queue is empty: transaction aborted to maintain consistency")
	}

	// [PUBLISH] Stage each event into the Outbox table within the current transaction.
	for _, event := range evs {
		// [ROUTING] Dynamic routing key generation based on the recipient's identity.
		topic := fmt.Sprintf("im_message.%s.message.%s.%s",
			event.RecipientID(),
			"created",
			s.getProtoVersion(),
		)

		if err := uow.Outbox().Publish(ctx, topic, event); err != nil {
			return fmt.Errorf("failed to stage outbox event for topic [%s]: %w", topic, err)
		}
	}

	return nil
}

// getProtoVersion extracts the version (e.g., "v1") from the gRPC ServiceName.
// ServiceName format: "webitel.im.internal.thread.v1.Message"
func (s *MessageService) getProtoVersion() string {
	parts := strings.Split(impb.Message_ServiceDesc.ServiceName, ".")
	for _, part := range parts {
		if len(part) >= 2 && part[0] == 'v' && part[1] >= '0' && part[1] <= '9' {
			return part
		}
	}
	return "v1"
}

// mapImageInputs transforms transport-layer DTOs into domain-layer inputs.
func (s *MessageService) mapImageInputs(dtoImages []*dto.Image) []model.ImageInput {
	inputs := make([]model.ImageInput, 0, len(dtoImages))
	for _, img := range dtoImages {
		inputs = append(inputs, model.ImageInput{
			FileID:   img.ID,
			MimeType: img.MimeType,
			Name:     "image_attachment",
		})
	}
	return inputs
}
