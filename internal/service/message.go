package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	guards "github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
)

type Messager interface {
	SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error)
	SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error)
	SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error)
}

type MessageService struct {
	uow            store.UnitOfWork
	logger         *slog.Logger
	threader       ThreadManager
	contactClient  imcontact.ContactsService
	mediaProcessor MediaProcessor
}

func NewMessageService(
	uow store.UnitOfWork,
	logger *slog.Logger,
	threader ThreadManager,
	contactClient imcontact.ContactsService,
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

// SendText handles normalization and multi-recipient distribution of text messages.
func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	// [VALIDATE] Ensure payload integrity
	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// [ACCESS_CONTROL] VERIFY communication permissions between peers via Contacts service
	cansend, err := s.contactClient.CanSend(ctx, dto.NewCanSendRequestDtoFromPeers(in.From, in.To, int32(in.DomainID)))
	if err != nil {
		s.logger.Error("error in can send rights validation gRPC request!", "err", err)
		return nil, err
	}

	// [GUARD] ENFORCE access policies and block unauthorized messaging attempts
	if err = guards.CanSendRightsViolationGuard(cansend.CanSend); err != nil {
		s.logger.Warn("send rights violation", "from", in.From.ID, "to", in.To.ID)
		return nil, err
	}

	// [THREAD] Resolve or initialize communication channel
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	// [MODEL] Construct domain entity with pre-staged distribution events
	msg := model.NewTextMessage(t.ID, in.From, []model.Peer{in.To}, in.Body)

	var resp *dto.SendTextResponse

	// [ATOMIC] Execute persistence and outbox staging in a single transaction
	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
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

	// [ACCESS_CONTROL] VERIFY communication permissions between peers via Contacts service
	cansend, err := s.contactClient.CanSend(ctx, dto.NewCanSendRequestDtoFromPeers(in.From, in.To, int32(in.DomainID)))
	if err != nil {
		s.logger.Error("error in can send rights validation gRPC request!", "err", err)
		return nil, err
	}

	// [GUARD] ENFORCE access policies and block unauthorized messaging attempts
	if err = guards.CanSendRightsViolationGuard(cansend.CanSend); err != nil {
		s.logger.Warn("send rights violation", "from", in.From.ID, "to", in.To.ID)
		return nil, err
	}

	// [THREAD]
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	// [CONVERT] TO ATTACHMENTS INTERFACE
	attachments := make([]AttachmentProcessor, len(in.Image.Images))
	for i, img := range in.Image.Images {
		attachments[i] = img
	}

	// [PROCESS] ALL IMAGES AT ONCE
	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
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
		// 1. [SAVE] Base message
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		// 2. [SAVE] Image attachments specifically
		if _, err := uow.Messages().SaveImages(txCtx, saved.ID, msg.Images); err != nil {
			return err
		}

		// 3. [DISPATCH]
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

// SendDocument processes document attachments and ensures transactional integrity.
func (s *MessageService) SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error) {
	// [VALIDATE]
	if err := guards.SendDocumentGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// [ACCESS_CONTROL] VERIFY communication permissions between peers via Contacts service
	cansend, err := s.contactClient.CanSend(ctx, dto.NewCanSendRequestDtoFromPeers(in.From, in.To, int32(in.DomainID)))
	if err != nil {
		s.logger.Error("error in can send rights validation gRPC request!", "err", err)
		return nil, err
	}

	// [GUARD] ENFORCE access policies and block unauthorized messaging attempts
	if err = guards.CanSendRightsViolationGuard(cansend.CanSend); err != nil {
		s.logger.Warn("send rights violation", "from", in.From.ID, "to", in.To.ID)
		return nil, err
	}

	// [THREAD] Ensure the communication channel exists
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &in.To,
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	// [CONVERT] TO ATTACHMENTS INTERFACE
	attachments := make([]AttachmentProcessor, len(in.Document.Documents))
	for i, img := range in.Document.Documents {
		attachments[i] = img
	}

	// [PROCESS] ALL IMAGES AT ONCE
	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
		return nil, err
	}

	// [MODEL] Initialize domain entity
	msg := model.NewDocumentMessage(
		t.ID,
		in.From,
		[]model.Peer{in.To},
		in.Document.Body,
		s.mapDocumentInputs(in.Document.Documents),
	)

	var resp *dto.SendDocumentResponse

	// [ATOMIC] Persistence and Outbox staging
	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		// 1. [SAVE] Base message
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return err
		}

		// 2. [SAVE] Document attachments specifically
		if _, err := uow.Messages().SaveDocuments(txCtx, saved.ID, msg.Documents); err != nil {
			return err
		}

		// 3. [DISPATCH]
		if err := s.dispatchEvents(txCtx, uow, msg); err != nil {
			return err
		}

		resp = &dto.SendDocumentResponse{ID: saved.ID, To: in.To}
		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "SEND_DOCUMENT_FAILED", "err", err)
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
			event.Version(),
		)

		if err := uow.Outbox().Publish(ctx, topic, event); err != nil {
			return fmt.Errorf("failed to stage outbox event for topic [%s]: %w", topic, err)
		}
	}

	return nil
}

// mapImageInputs transforms transport-layer DTOs into domain-layer inputs.
func (s *MessageService) mapImageInputs(dtoImages []*dto.Image) []model.ImageInput {
	inputs := make([]model.ImageInput, 0, len(dtoImages))
	for _, img := range dtoImages {
		inputs = append(inputs, model.ImageInput{
			FileID:   strconv.FormatInt(img.ID, 10),
			MimeType: img.MimeType,
		})
	}
	return inputs
}

func (s *MessageService) mapDocumentInputs(dtoDocs []*dto.Document) []model.DocumentInput {
	inputs := make([]model.DocumentInput, 0, len(dtoDocs))
	for _, doc := range dtoDocs {
		inputs = append(inputs, model.DocumentInput{
			// [CONVERT] int64 to string
			FileID:   strconv.FormatInt(doc.ID, 10),
			Name:     doc.Name,
			MimeType: doc.MimeType,
			Size:     doc.Size,
		})
	}
	return inputs
}
