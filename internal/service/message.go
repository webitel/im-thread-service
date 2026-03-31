package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	guards "github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type Messager interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
	SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error)
	SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error)
	Read(ctx context.Context, in *dto.ReadMessageRequest) error
}

type MessageService struct {
	uow            store.UnitOfWork
	logger         *slog.Logger
	threader       ThreadManager
	contactClient  *imcontact.Client
	mediaProcessor MediaProcessor
}

func NewMessageService(
	uow store.UnitOfWork,
	logger *slog.Logger,
	threader ThreadManager,
	contactClient *imcontact.Client,
	mediaProcessor MediaProcessor,
) *MessageService {
	return &MessageService{
		uow:            uow,
		logger:         logger,
		threader:       threader,
		contactClient:  contactClient,
		mediaProcessor: mediaProcessor,
	}
}

var _ Messager = (*MessageService)(nil)

func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	log := s.logger.With("operation", "message.SendText")

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		log.Error(
			"failed to ensure direct thread", "err", err,
			slog.Any("from", in.From),
			slog.Any("to", in.To),
			slog.Any("domain_id", in.DomainID),
			slog.String("member_id", in.From.ID.String()),
		)

		return nil, err
	}

	msg := &model.Message{
		ThreadID: t.ID,
		DomainID: int32(in.DomainID),
		From:     in.From,
		Body:     in.Body,
		To:       t.Members,
		Type:     model.MessageTypeText,
		SenderID: in.From.ID,
		Metadata: model.BuildMetadata(in.Body),
	}

	err = s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(ctx, msg)
		if err != nil {
			return err
		}

		saved.To = t.Members
		saved.WithCreatedEvent(in.SendID)

		if err = s.dispatchEvents(ctx, uow, saved); err != nil {
			return err
		}

		msg = saved

		return nil
	})

	if err != nil {
		log.ErrorContext(
			ctx,
			"error saving text message",
			slog.Any("error", err),
			slog.String("thread_id", t.ID.String()),
			slog.String("from", msg.From.ID.String()),
		)

		return nil, errors.Internal("error saving text message", errors.WithCause(err))
	}

	return &dto.SendTextResponse{ID: msg.ID, To: in.To}, nil
}

func (s *MessageService) SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error) {
	if err := guards.SendImageGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	attachments := make([]AttachmentProcessor, len(in.Image.Images))
	for i, img := range in.Image.Images {
		attachments[i] = img
	}
	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
		return nil, err
	}

	msg := model.NewImageMessage(model.MessageCreate{
		ThreadID:   t.ID,
		DomainID:   int32(in.DomainID),
		From:       in.From,
		Recipients: t.Members,
		Body:       in.Image.Body,
		SendID:     in.SendID,
		Images:     s.mapImageInputs(in.Image.Images),
	})

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}
		if _, err := uow.Messages().SaveImages(txCtx, saved.ID, msg.Images); err != nil {
			return err
		}

		msg.ID = saved.ID

		return s.dispatchEvents(txCtx, uow, msg)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_image_failed", "err", err)
		return nil, err
	}

	return &dto.SendImageResponse{ID: msg.ID, To: in.To}, nil
}

// SendDocument processes document attachments and ensures transactional integrity.
func (s *MessageService) SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error) {
	if err := guards.SendDocumentGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	attachments := make([]AttachmentProcessor, len(in.Document.Documents))
	for i, doc := range in.Document.Documents {
		attachments[i] = doc
	}
	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
		return nil, err
	}

	msg := model.NewDocumentMessage(model.MessageCreate{
		ThreadID:   t.ID,
		DomainID:   int32(in.DomainID),
		From:       in.From,
		Recipients: t.Members,
		Body:       in.Document.Body,
		SendID:     in.SendID,
		Documents:  s.mapDocumentInputs(in.Document.Documents),
	})

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}
		if _, err := uow.Messages().SaveDocuments(txCtx, saved.ID, msg.Documents); err != nil {
			return err
		}

		msg.ID = saved.ID

		return s.dispatchEvents(txCtx, uow, msg)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_document_failed", "err", err)
		return nil, err
	}

	return &dto.SendDocumentResponse{ID: msg.ID, To: in.To}, nil
}

func (s *MessageService) Read(ctx context.Context, in *dto.ReadMessageRequest) error {
	if err := guards.ValidateReadMessage(in); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	tID, _ := uuid.Parse(in.ThreadID)
	mID, _ := uuid.Parse(in.MessageID)
	uID, _ := uuid.Parse(in.UserID)

	return s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		err := uow.Messages().ReadMessage(txCtx, struct {
			DomainID  int32
			ThreadID  uuid.UUID
			MessageID uuid.UUID
			UserID    uuid.UUID
		}{
			DomainID:  in.DomainID,
			ThreadID:  tID,
			MessageID: mID,
			UserID:    uID,
		})
		if err != nil {
			return fmt.Errorf("read_message: %w", err)
		}

		return nil
	})
}

// --- Internal Helpers ---

func (s *MessageService) executeMessageTransaction(ctx context.Context, msg *model.Message) error {
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var (
			savedMsg *model.Message
			err      error
		)
		if savedMsg, err = uow.Messages().SaveMessage(txCtx, msg); err != nil {
			return err
		}

		msg.ID = savedMsg.ID

		return s.dispatchEvents(txCtx, uow, msg)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "transaction_failed", "err", err)
	}
	return err
}

func (s *MessageService) dispatchEvents(ctx context.Context, uow store.UnitOfWork, msg *model.Message) error {
	evs := msg.Events()
	if len(evs) == 0 {
		return fmt.Errorf("domain events queue is empty: transaction aborted")
	}

	for _, event := range evs {
		topic := fmt.Sprintf("im_message.%s.message.%s.%s",
			event.RecipientID(),
			"created",
			event.Version(),
		)

		if err := uow.Outbox().Publish(ctx, topic, event); err != nil {
			return fmt.Errorf("outbox publish failed: %w", err)
		}
	}
	return nil
}

func (s *MessageService) mapImageInputs(dtoImages []*dto.Image) []model.ImageInput {
	inputs := make([]model.ImageInput, 0, len(dtoImages))
	for _, img := range dtoImages {
		inputs = append(inputs, model.ImageInput{
			FileID:   strconv.FormatInt(img.ID, 10),
			Name:     img.Name,
			URL:      img.URL,
			MimeType: img.MimeType,
		})
	}
	return inputs
}

func (s *MessageService) mapDocumentInputs(dtoDocs []*dto.Document) []model.DocumentInput {
	inputs := make([]model.DocumentInput, 0, len(dtoDocs))
	for _, doc := range dtoDocs {
		inputs = append(inputs, model.DocumentInput{
			FileID:   strconv.FormatInt(doc.ID, 10),
			Name:     doc.Name,
			MimeType: doc.MimeType,
			Size:     doc.Size,
		})
	}
	return inputs
}
