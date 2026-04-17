package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	guards "github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type ThreadManager interface {
	EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error)
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

func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	log := s.logger.With("operation", "message.SendText")

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		From:     &in.From,
		To:       &in.To,
		DomainID: int(in.DomainID),
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
		saved.WithCreatedEvent(in.SendID, &in.From)

		if err = s.dispatchMessageEvents(ctx, uow, saved); err != nil {
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
		From:     &in.From,
		To:       &in.To,
		DomainID: int(in.DomainID),
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

	fileLinksChan := s.mediaProcessor.FetchFileLinks(ctx, in.DomainID, attachments)
	if err := enrichAttachmentsLinks(ctx, attachments, fileLinksChan); err != nil {
		s.logger.ErrorContext(ctx, "enriching attachments links", "err", err)
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
			return errors.Internal("save message", errors.WithCause(err), errors.WithID("service.message.send_image"))
		}
		if _, err := uow.Messages().SaveImages(txCtx, saved.ID, msg.Images); err != nil {
			return errors.Internal("save images", errors.WithCause(err), errors.WithID("service.message.send_image"))
		}

		msg.ID = saved.ID
		msg.WithCreatedEvent(in.SendID, &in.From)

		if err := s.dispatchMessageEvents(txCtx, uow, msg); err != nil {
			return errors.Internal("dispatch message events", errors.WithCause(err), errors.WithID("service.message.send_image"))
		}

		return nil
	})

	if err != nil {
		s.logger.ErrorContext(ctx, "send image failed", "err", err)
		return nil, err
	}

	return &dto.SendImageResponse{ID: msg.ID, To: in.To}, nil
}

func (s *MessageService) SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error) {
	if err := guards.SendDocumentGuard(in); err != nil {
		return nil, errors.InvalidArgument("send document request validation", errors.WithCause(err), errors.WithID("service.message.send_document"))
	}

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		From:     &in.From,
		To:       &in.To,
		DomainID: int(in.DomainID),
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

	fileLinksChan := s.mediaProcessor.FetchFileLinks(ctx, in.DomainID, attachments)
	if err := enrichAttachmentsLinks(ctx, attachments, fileLinksChan); err != nil {
		s.logger.ErrorContext(ctx, "enriching attachments links", "err", err)
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
			return errors.Internal("save message", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		if _, err := uow.Messages().SaveDocuments(txCtx, saved.ID, msg.Documents); err != nil {
			return errors.Internal("save documents", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		msg.ID = saved.ID
		msg.WithCreatedEvent(in.SendID, &in.From)

		if err := s.dispatchMessageEvents(txCtx, uow, msg); err != nil {
			return errors.Internal("dispatch message events", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		return nil
	})

	if err != nil {
		s.logger.ErrorContext(ctx, "send document failed", "err", err)
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

func (s *MessageService) SendLocation(ctx context.Context, msg *model.Message) (*model.Message, error) {
	log := s.logger.With("operation", "send_location")

	if err := s.prepareMessageForSending(ctx, msg); err != nil {
		log.ErrorContext(ctx, "prepare_message_failed", "err", err)
		return nil, err
	}

	var savedMsg *model.Message
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var err error
		if savedMsg, err = uow.Messages().SaveMessageLocation(txCtx, msg); err != nil {
			return err
		}
		savedMsg.To = msg.To
		savedMsg.IdempotencyKey = msg.IdempotencyKey

		savedMsg.WithCreatedEvent(msg.IdempotencyKey, &msg.From)

		return s.dispatchMessageEvents(txCtx, uow, savedMsg)
	})

	if err != nil {
		log.ErrorContext(ctx, "location_transaction_failed", "err", err)
		return nil, err
	}

	return savedMsg, err
}

func (s *MessageService) SendContact(ctx context.Context, msg *model.Message) (*model.Message, error) {
	log := s.logger.With("operation", "send_contact")

	if err := s.prepareMessageForSending(ctx, msg); err != nil {
		log.ErrorContext(ctx, "prepare_message_failed", "err", err)
		return nil, err
	}

	var savedMsg *model.Message
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var err error
		if savedMsg, err = uow.Messages().SaveMessageContact(txCtx, msg); err != nil {
			return err
		}
		savedMsg.To = msg.To
		savedMsg.IdempotencyKey = msg.IdempotencyKey

		savedMsg.WithCreatedEvent(msg.IdempotencyKey, &msg.From)

		return s.dispatchMessageEvents(txCtx, uow, savedMsg)
	})

	if err != nil {
		log.ErrorContext(ctx, "contact_transaction_failed", "err", err)
		return nil, err
	}

	return savedMsg, err
}

func (s *MessageService) SendInteractive(ctx context.Context, msg *model.Message) (*model.Message, error) {
	log := s.logger.With("operation", "send_interactive")

	if err := s.prepareMessageForSending(ctx, msg); err != nil {
		log.ErrorContext(ctx, "prepare_message_failed", "err", err)
		return nil, err
	}

	if len(msg.Documents) > 0 || len(msg.Images) > 0 {
		attachments := make([]AttachmentProcessor, 0, len(msg.Documents)+len(msg.Images))
		for _, doc := range msg.Documents {
			attachments = append(attachments, doc)
		}
		for _, img := range msg.Images {
			attachments = append(attachments, img)
		}

		if err := s.mediaProcessor.Process(ctx, int64(msg.DomainID), attachments); err != nil {
			log.ErrorContext(ctx, "media_process_failed", "err", err)
			return nil, errors.Internal(
				"media_process_failed",
				errors.WithCause(err),
				errors.WithID("service.message.save_interactive.media_process_failed"),
			)
		}
	}

	var savedMsg *model.Message
	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		var err error
		if savedMsg, err = uow.Messages().SaveInteractiveMessage(ctx, msg); err != nil {
			return err
		}

		savedMsg.To = msg.To
		savedMsg.WithCreatedEvent(msg.IdempotencyKey, &msg.From)

		return s.dispatchMessageEvents(ctx, uow, savedMsg)
	})

	if err != nil {
		log.ErrorContext(ctx, "transaction_failed", "err", err)
		return nil, err
	}

	return savedMsg, nil
}

func (s *MessageService) SendInteractiveCallback(ctx context.Context, callback *model.InteractiveCallback) (*model.InteractiveCallback, error) {
	log := s.logger.With("operation", "send_interactive_callback")

	var savedCallback *model.InteractiveCallback
	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		var err error
		if savedCallback, err = uow.InteractiveCallback().Save(ctx, callback); err != nil {
			return err
		}

		savedCallback.WithCreatedEvent()

		return s.dispatchInteractiveCallbackEvents(ctx, uow, savedCallback)
	})

	if err != nil {
		log.ErrorContext(
			ctx,
			"save_failed",
			"err", err,
			"reacted_by", callback.ReactedBy,
			"in_reply_to", callback.InReplyTo,
			"button_code", callback.ButtonCode,
		)
		return nil, err
	}

	return savedCallback, nil
}

func (s *MessageService) SendSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	log := s.logger.With("operation", "send_system_message")

	if err := s.prepareMessageForSending(ctx, msg); err != nil {
		log.ErrorContext(ctx, "prepare_message_failed", "err", err)
		return nil, err
	}

	var savedMsg *model.Message
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var err error
		if savedMsg, err = uow.Messages().SaveSystemMessage(txCtx, msg); err != nil {
			return err
		}
		savedMsg.To = msg.To
		savedMsg.IdempotencyKey = msg.IdempotencyKey

		savedMsg.WithCreatedEvent(msg.IdempotencyKey, &msg.From)

		return s.dispatchMessageEvents(txCtx, uow, savedMsg)
	})

	if err != nil {
		log.ErrorContext(ctx, "transaction_failed", "err", err)
		return nil, err
	}

	return savedMsg, nil
}

func (s *MessageService) prepareMessageForSending(ctx context.Context, msg *model.Message) error {
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		DomainID: int(msg.DomainID),
		From:     &msg.From,
		To:       &msg.SendTo,
	})

	if err != nil {
		return err
	}

	msg.ThreadID = t.ID
	msg.To = t.Members

	return nil
}

// --- Internal Helpers ---
func (s *MessageService) dispatchInteractiveCallbackEvents(ctx context.Context, uow store.UnitOfWork, callback *model.InteractiveCallback) error {
	evs := callback.Events()
	if len(evs) == 0 {
		s.logger.Warn("service.message.dispatchInteractiveCallbackEvents: no events to dispatch")
		return nil
	}

	return s.dispatchEvents(ctx, uow, evs, func(e event.Outboxer) string {
		return fmt.Sprintf("im_message.%s.interactive_callback.%s.%s",
			e.RecipientID(),
			e.EventType(),
			e.Version(),
		)
	})
}

func (s *MessageService) dispatchMessageEvents(ctx context.Context, uow store.UnitOfWork, msg *model.Message) error {
	evs := msg.Events()
	if len(evs) == 0 {
		return fmt.Errorf("domain events queue is empty: transaction aborted")
	}

	return s.dispatchEvents(ctx, uow, evs, func(e event.Outboxer) string {
		return fmt.Sprintf("im_message.%s.message.%s.%s",
			e.RecipientID(),
			"created",
			e.Version(),
		)
	})
}

func (s *MessageService) dispatchEvents(ctx context.Context, uow store.UnitOfWork, eventss []event.Outboxer, topicCallback func(event.Outboxer) string) error {
	for _, event := range eventss {
		topic := topicCallback(event)

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
			URL:      doc.URL,
		})
	}
	return inputs
}

func enrichAttachmentsLinks(ctx context.Context, attachments []AttachmentProcessor, fileLinksChan <-chan fetchLinksResult) error {
	if len(attachments) == 0 {
		return nil
	}

	select {
	case result, ok := <-fileLinksChan:
		if !ok {
			return errors.Aborted("accessing closed links chan", errors.WithID("service.message.enrich_attachments_links"))
		}

		if result.Err != nil {
			return result.Err
		}

		for id, url := range result.FileLinks {
			for i := range attachments {
				if attachments[i].GetID() == id {
					attachments[i].SetURL(url)
					break
				}
			}
		}
	case <-ctx.Done():
		return errors.Aborted("context cancelled", errors.WithID("service.message.enrich_attachments_links"), errors.WithCause(ctx.Err()))
	}

	return nil
}
