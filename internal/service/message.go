package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	impb "github.com/webitel/im-thread-service/gen/go/contact/v1"
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
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

// SendText handles normalization and multi-recipient distribution of text messages.
func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	// [VALIDATE] ENSURE PAYLOAD INTEGRITY
	if err := guards.SendTextGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// [RESOLVE] EXECUTE PERMISSIONS CHECK AND RECIPIENT IDENTITY LOOKUP
	toID, err := s.resolveRecipient(ctx, in.From, in.To, int32(in.DomainID))
	if err != nil {
		return nil, err
	}

	// [THREAD] RESOLVE OR INITIALIZE COMMUNICATION CHANNEL
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &shared.Peer{ID: toID},
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	// [MODEL] CONSTRUCT DOMAIN ENTITY
	msg := model.NewTextMessage(t.ID, int32(in.DomainID), in.From, []shared.Peer{{ID: toID}}, in.Body)

	// [ATOMIC] EXECUTE PERSISTENCE AND DISPATCH WITHIN TRANSACTION
	if err := s.executeMessageTransaction(ctx, msg); err != nil {
		return nil, err
	}

	return &dto.SendTextResponse{ID: msg.ID, To: in.To}, nil
}

// SendImage handles media attachments and transactional event propagation.
func (s *MessageService) SendImage(ctx context.Context, in *dto.SendImageRequest) (*dto.SendImageResponse, error) {
	// [VALIDATE] CHECK MEDIA CONSTRAINTS
	if err := guards.SendImageGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// [RESOLVE] VERIFY ACCESS AND FIND RECIPIENT
	toID, err := s.resolveRecipient(ctx, in.From, in.To, int32(in.DomainID))
	if err != nil {
		return nil, err
	}

	// [THREAD] ENSURE CHANNEL EXISTENCE
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &shared.Peer{ID: toID},
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	// [PROCESS] CONVERT AND UPLOAD MEDIA ATTACHMENTS
	attachments := make([]AttachmentProcessor, len(in.Image.Images))
	for i, img := range in.Image.Images {
		attachments[i] = img
	}
	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
		return nil, err
	}

	// [MODEL] INITIALIZE IMAGE DOMAIN ENTITY
	msg := model.NewImageMessage(
		t.ID,
		int32(in.DomainID),
		in.From,
		[]shared.Peer{{ID: toID}},
		in.Image.Body,
		s.mapImageInputs(in.Image.Images),
	)

	// [ATOMIC] SAVE MESSAGE AND ATTACHMENTS
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

// SendDocument processes document attachments and ensures transactional integrity.
func (s *MessageService) SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error) {
	// [VALIDATE]
	if err := guards.SendDocumentGuard(in); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	// [RESOLVE]
	toID, err := s.resolveRecipient(ctx, in.From, in.To, int32(in.DomainID))
	if err != nil {
		return nil, err
	}

	// [THREAD]
	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		PeerFrom: &in.From,
		PeerTo:   &shared.Peer{ID: toID},
		DomainID: int(in.DomainID),
		MemberID: in.From.ID,
	})
	if err != nil {
		return nil, err
	}

	// [PROCESS] MEDIA
	attachments := make([]AttachmentProcessor, len(in.Document.Documents))
	for i, doc := range in.Document.Documents {
		attachments[i] = doc
	}
	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
		return nil, err
	}

	// [MODEL]
	msg := model.NewDocumentMessage(
		t.ID,
		int32(in.DomainID),
		in.From,
		[]shared.Peer{{ID: toID}},
		in.Document.Body,
		s.mapDocumentInputs(in.Document.Documents),
	)

	// [ATOMIC]
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

// --- Internal Helpers ---

// [INTERNAL] RESOLVEREPCIPIENT HANDLES PERMISSIONS AND IDENTITY DISCOVERY
func (s *MessageService) resolveRecipient(ctx context.Context, from, to shared.Peer, domainID int32) (uuid.UUID, error) {
	// CHECK COMMUNICATION RIGHTS
	cansend, err := s.contactClient.CanSend(ctx, dto.NewCanSendRequestDtoFromPeers(from, to, domainID))
	if err != nil {
		s.logger.Error("rights validation failed", "err", err)
		return uuid.Nil, err
	}
	if err = guards.CanSendRightsViolationGuard(cansend.CanSend); err != nil {
		return uuid.Nil, err
	}

	// FIND INTERNAL CONTACT IDENTITY
	out, err := s.contactClient.SearchContact(ctx, &impb.SearchContactRequest{Subjects: []string{to.ID.String()}, DomainId: domainID})
	if err != nil {
		return uuid.Nil, fmt.Errorf("contact search failed: %w", err)
	}
	if len(out.Contacts) == 0 {
		return uuid.Nil, fmt.Errorf("recipient contact not found")
	}

	return uuid.Parse(out.Contacts[0].Id)
}

// [INTERNAL] EXECUTEMESSAGETRANSACTION HANDLES BASE MESSAGE PERSISTENCE AND EVENT DISPATCH
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

// dispatchEvents handles the propagation of staged domain events to the persistent Outbox.
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

// mapImageInputs transforms transport-layer DTOs into domain-layer inputs.
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

// mapDocumentInputs transforms DTOs to domain models.
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
