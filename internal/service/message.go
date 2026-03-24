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
	SendLocation(ctx context.Context, msg *model.Message) (*model.Message, error)
	SendContact(ctx context.Context, msg *model.Message) (*model.Message, error)
	SendInteractive(ctx context.Context, msg *model.Message) (*model.Message, error)
	Threader() ThreadManager
	HandleInteractiveCallback(ctx context.Context, callback *model.ButtonsCallback) (*model.ButtonsCallback, error)
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

func (s *MessageService) Threader() ThreadManager { return s.threader }

// SendText handles normalization and multi-recipient distribution of text messages.
func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	if err := guards.SendTextGuard(in); err != nil {
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

	// [CLEAN] Using named structure instead of positional arguments
	msg := model.NewTextMessage(model.MessageCreate{
		ThreadID:   t.ID,
		DomainID:   int32(in.DomainID),
		From:       in.From,
		Recipients: t.Members,
		Body:       in.Body,
		SendID:     in.SendID,
	})

	if err := s.executeMessageTransaction(ctx, msg); err != nil {
		return nil, err
	}

	return &dto.SendTextResponse{ID: msg.ID, To: in.To}, nil
}

// SendImage handles media attachments and transactional event propagation.
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

	// [CLEAN] Initializing image message with clear intent
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
		return s.dispatchEvents(txCtx, uow, msg)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_image_failed", "err", err)
		return nil, err
	}

	return &dto.SendImageResponse{ID: msg.ID, To: in.To}, nil
}

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
		return s.dispatchEvents(txCtx, uow, msg)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send_document_failed", "err", err)
		return nil, err
	}

	return &dto.SendDocumentResponse{ID: msg.ID, To: in.To}, nil
}

func (s *MessageService) Read(ctx context.Context, in *dto.ReadMessageRequest) error {
	// 1. Validate basic constraints
	if err := guards.ValidateReadMessage(in); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	// 2. Parse string IDs to UUIDs
	tID, _ := uuid.Parse(in.ThreadID)
	mID, _ := uuid.Parse(in.MessageID)
	uID, _ := uuid.Parse(in.UserID)

	// 3. Execute logic
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		// Pass the anonymous struct as a single argument
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

func (s *MessageService) SendLocation(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := s.prepareMessage(ctx, msg); err != nil {
		s.logger.WarnContext(ctx, "error preparing message", slog.Any("error", err))
		return nil, err
	}
	
	log := s.logger.With(
		slog.String("op", "SendLocation"),
		slog.String("thread_id", msg.ThreadID.String()),
		slog.Int("domain_id", int(msg.DomainID)),
		slog.String("from", msg.From.ID.String()),
	)

	var (
		savedMsg *model.Message
		err      error
	)

	err = s.uow.WithinTransaction(
		ctx,
		func(ctx context.Context, uow store.UnitOfWork) error {
			if savedMsg, err = uow.Messages().SaveLocation(ctx, msg); err != nil {
				log.ErrorContext(
					ctx,
					"error saving message with location",
					slog.Any("error", err),
				)

				return err
			}

			savedMsg.WithCreatedEvent()

			if err = s.dispatchEvents(ctx, uow, savedMsg); err != nil {
				log.ErrorContext(
					ctx,
					"error dispatching events to DB",
					slog.Any("error", err),
				)

				return err
			}

			return nil
		},
	)

	if err != nil {
		return nil, errors.Internal(
			"error processing SendLocation request",
			errors.WithCause(err),
		)
	}

	return savedMsg, nil
}

func (s *MessageService) SendContact(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := s.prepareMessage(ctx, msg); err != nil {
		s.logger.WarnContext(ctx, "error preparing message", slog.Any("error", err))
		return nil, err
	}

	log := s.logger.With(
		slog.String("op", "SendLocation"),
		slog.String("thread_id", msg.ThreadID.String()),
		slog.String("from", msg.From.ID.String()),
	)

	var saved *model.Message
	err := s.uow.WithinTransaction(
		ctx,
		func(ctx context.Context, uow store.UnitOfWork) error {
			processedMsg, err := uow.Messages().SaveContact(ctx, msg)
			if err != nil {
				return err
			}

			processedMsg.WithCreatedEvent()

			if err = s.dispatchEvents(ctx, uow, processedMsg); err != nil {
				return err
			}

			saved = processedMsg

			return err
		},
	)

	if err != nil {
		log.ErrorContext(
			ctx,
			"error saving contact message",
			slog.Any("error", err),
		)

		return nil, err
	}

	return saved, nil
}

func (s *MessageService) SendInteractive(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := s.prepareMessage(ctx, msg); err != nil {
		s.logger.WarnContext(ctx, "error preparing message", slog.Any("error", err))
		return nil, err
	}
	
	log := s.logger.With(
		slog.String("op", "SendInteractive"),
		slog.String("thread_id", msg.ThreadID.String()),
		slog.Int("domain_id", int(msg.DomainID)),
		slog.String("from", msg.From.ID.String()),
	)

	if err := s.prepareInteractiveHeaderContent(ctx, msg.Interactive.Header, int64(msg.DomainID)); err != nil {
		log.ErrorContext(ctx, "error preparing message header content via storage", slog.Any("error", err))
		return nil, err
	}

	var savedMsg *model.Message

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		var err error
		if savedMsg, err = uow.Messages().SaveMessage(ctx, msg); err != nil {
			return err
		}

		if err = s.processInteractiveHeaderContent(ctx, uow, savedMsg, msg.Interactive.Header); err != nil {
			return err
		}

		savedMsg.SendTo = msg.SendTo

		savedMsg.WithCreatedEvent()

		if err = s.dispatchEvents(ctx, uow, savedMsg); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.ErrorContext(
			ctx,
			"error saving interactive message",
			slog.Any("error", err),
		)

		return nil, err
	}

	return savedMsg, nil
}

func (s *MessageService) HandleInteractiveCallback(ctx context.Context, callback *model.ButtonsCallback) (*model.ButtonsCallback, error) {
	log := s.logger.With(
		slog.String("op", "HandleInteractiveCallback"),
		slog.String("message_id", callback.MessageID.String()),
		slog.String("button_code", callback.ButtonCode),
		slog.String("clicked_by", callback.ClickedBy.String()),
	)

	var saved, err = s.uow.ButtonsCallback().Create(ctx, callback)
	if err != nil {
		log.ErrorContext(ctx, "error saving callback response", slog.Any("error", err))
		return nil, err
	}

	return saved, nil
}

// #region Private

func (s *MessageService) prepareInteractiveHeaderContent(ctx context.Context, header *model.InteractiveHeader, domainID int64) error {
	var (
		imagesLen    = len(header.Images)
		documentsLen = len(header.Images)
		attachments  = make([]AttachmentProcessor, 0, documentsLen+imagesLen)
	)

	if imagesLen > 0 {
		attachments = append(attachments, toAttachments(header.Images)...)
	}

	if documentsLen > 0 {
		attachments = append(attachments, toAttachments(header.Documents)...)
	}

	if err := s.mediaProcessor.Process(ctx, domainID, attachments); err != nil {
		return err
	}

	return nil
}

func (s *MessageService) processInteractiveHeaderContent(ctx context.Context, uow store.UnitOfWork, saved *model.Message, header *model.InteractiveHeader) error {
	if len(header.Documents) > 0 {
		var savedDocuments, err = uow.Messages().SaveDocuments(ctx, saved.ID, header.Documents)
		if err != nil {
			return err
		}

		saved.Documents = savedDocuments
	}

	if len(header.Images) > 0 {
		var savedImages, err = uow.Messages().SaveImages(ctx, saved.ID, header.Images)
		if err != nil {
			return err
		}

		saved.Images = savedImages
	}

	return nil
}

func (s *MessageService) executeMessageTransaction(ctx context.Context, msg *model.Message) error {
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		if _, err := uow.Messages().SaveMessage(txCtx, msg); err != nil {
			return err
		}
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

func toAttachments[T AttachmentProcessor](attachments []T) []AttachmentProcessor {
	var converted []AttachmentProcessor = make([]AttachmentProcessor, len(attachments))
	for i, att := range attachments {
		converted[i] = att
	}

	return converted
}

func (s *MessageService) prepareMessage(ctx context.Context, msg *model.Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	thread, err := s.threader.EnsureDirectThread(
		ctx,
		&dto.EnsureDirectThreadRequest{
			DomainID: msg.GetDomainID(),
			MemberID: msg.From.ID,
			PeerFrom: &msg.From,
			PeerTo:   &msg.SendTo,
		},
	)

	if err != nil {
		return err
	}

	msg.SetThread(thread.ID, thread.Members)
	
	return nil
}

// #endregion
