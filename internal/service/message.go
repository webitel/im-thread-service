package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
)

type ThreadManager interface {
	EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error)
	ReleaseBotControl(ctx context.Context, req *dto.ReleaseBotControlRequest) error
}

const botStopCommand = "/close"

func isStopCommand(body string) bool {
	return strings.TrimSpace(body) == botStopCommand
}

type MessageService struct {
	uow              store.UnitOfWork
	logger           *slog.Logger
	threader         ThreadManager
	contactClient    *imcontact.Client
	mediaProcessor   MediaProcessor
	providersAdapter ProvidersAdapter
}

func NewMessageService(
	uow store.UnitOfWork,
	logger *slog.Logger,
	threader ThreadManager,
	contactClient *imcontact.Client,
	mediaProcessor MediaProcessor,
	providersAdapter ProvidersAdapter,
) *MessageService {
	return &MessageService{
		uow:              uow,
		logger:           logger,
		threader:         threader,
		contactClient:    contactClient,
		mediaProcessor:   mediaProcessor,
		providersAdapter: providersAdapter,
	}
}

func (s *MessageService) sendMessageToExternalProvider(ctx context.Context, message *model.Message) error {
	return s.providersAdapter.SendMessage(ctx, message)
}

// resolveToIsBot checks whether the target contact is a bot.
// Returns false on error to remain non-blocking — the thread will be created without bot control.
func (s *MessageService) resolveToIsBot(ctx context.Context, toID uuid.UUID, domainID int) bool {
	if s.contactClient == nil {
		s.logger.WarnContext(ctx, "resolveToIsBot: contactClient is nil, assuming false", "contact_id", toID)

		return false
	}

	isBot, err := s.contactClient.IsBot(ctx, toID, domainID)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to resolve is_bot for to peer, assuming false", "contact_id", toID, "err", err)

		return false
	}

	s.logger.DebugContext(ctx, "resolveToIsBot result", "contact_id", toID, "domain_id", domainID, "is_bot", isBot)

	return isBot
}

func (s *MessageService) SendText(ctx context.Context, in *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	if err := guards.SendTextGuard(in); err != nil {
		return nil, errors.InvalidArgument("validating text message", errors.WithCause(err), errors.WithID("service.message.send_text"))
	}

	log := s.logger.With("operation", "message.SendText")

	{
		toVia := "<nil>"
		if in.To.Identity != nil && in.To.Identity.Via != nil {
			toVia = *in.To.Identity.Via
		}

		log.Debug("incoming request",
			slog.String("to_id", in.To.ID.String()),
			slog.String("to_via", toVia),
		)
	}

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		From:     &in.From,
		To:       &in.To,
		DomainID: int(in.DomainID),
		SendAs:   in.SendAs,
		ToIsBot:  func() bool { return s.resolveToIsBot(ctx, in.To.ID, int(in.DomainID)) },
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

	for i, m := range t.Members {
		via := "<nil>"
		if m.Via != nil {
			via = *m.Via
		}

		log.Debug("thread member after EnsureDirectThread",
			slog.Int("index", i),
			slog.String("contact_id", m.ContactID.String()),
			slog.String("via", via),
		)
	}

	if isStopCommand(in.Body) && s.shouldStopBot(t, in.From.ID) {
		return s.handleBotStopCommand(ctx, in, t)
	}

	replyPreview, err := s.resolveReply(ctx, in.ReplyToMessageID, in.ReplyToExternalID, &in.From, t, int32(in.DomainID))
	if err != nil {
		return nil, err
	}

	msg := &model.Message{
		ThreadID:              t.ID,
		DomainID:              int32(in.DomainID),
		From:                  in.From,
		Body:                  in.Body,
		To:                    t.Members,
		Type:                  model.MessageTypeText,
		Metadata:              model.BuildMetadata(in.Body),
		SendAs:                in.SendAs,
		BotControllerMemberID: t.BotControllerID,
		ReplyTo:               replyPreview,
	}

	msg.SetMemberFromSlice(t.Members)

	err = s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(ctx, msg)
		if err != nil {
			return err
		}

		saved.To = t.Members
		saved.ReplyTo = msg.ReplyTo

		if err = s.recordInboundExternalID(ctx, uow, saved, &in.From, in.ExternalID); err != nil {
			return err
		}

		saved.WithCreatedEvent(ctx, in.SendID)

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

		return nil, errors.Internal("error saving text message", errors.WithCause(err), errors.WithID("service.message.send_text"))
	}

	if err = s.sendMessageToExternalProvider(ctx, msg); err != nil {
		log.Error("sending text message to external providers", "error", err)
	}

	return &dto.SendTextResponse{ID: msg.ID, To: in.To}, nil
}

const botStoppedSystemType = "bot_stopped"

func (s *MessageService) shouldStopBot(t *model.Thread, fromContactID uuid.UUID) bool {
	if t == nil || t.BotControllerID == nil {
		return false
	}

	if sender := memberByContactID(t.Members, fromContactID); sender != nil {
		if sender.IsBot || sender.ID == *t.BotControllerID {
			return false
		}
	}

	return true
}

func (s *MessageService) handleBotStopCommand(ctx context.Context, in *dto.SendTextRequest, t *model.Thread) (*dto.SendTextResponse, error) {
	log := s.logger.With("operation", "message.handleBotStopCommand", slog.String("thread_id", t.ID.String()))

	var initiatorMemberID uuid.UUID
	if sender := memberByContactID(t.Members, in.From.ID); sender != nil {
		initiatorMemberID = sender.ID
	}

	if err := s.threader.ReleaseBotControl(ctx, &dto.ReleaseBotControlRequest{
		ThreadID:          t.ID,
		InitiatorMemberID: initiatorMemberID,
		DomainID:          int(in.DomainID),
	}); err != nil {
		log.ErrorContext(ctx, "failed to release bot control on /close", "err", err)

		return nil, err
	}

	msg := s.buildBotStoppedMessage(in, t)

	var saved *model.Message

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var e error
		if saved, e = uow.Messages().SaveSystemMessage(txCtx, msg); e != nil {
			return e
		}

		saved.To = msg.To
		saved.WithCreatedEvent(ctx, msg.IdempotencyKey)

		return s.dispatchMessageEvents(txCtx, uow, saved)
	})
	if err != nil {
		log.ErrorContext(ctx, "failed to send bot stopped system message", "err", err)

		return &dto.SendTextResponse{To: in.To}, nil
	}

	log.InfoContext(ctx, "bot stopped via /close", slog.String("system_message_id", saved.ID.String()))

	return &dto.SendTextResponse{ID: saved.ID, To: in.To}, nil
}

func (s *MessageService) buildBotStoppedMessage(in *dto.SendTextRequest, t *model.Thread) *model.Message {
	to := make([]*model.ThreadDialog, 0, len(t.Members))
	for _, m := range t.Members {
		if m != nil && !m.IsBot {
			to = append(to, m)
		}
	}

	msg := &model.Message{
		ThreadID:       t.ID,
		DomainID:       int32(in.DomainID),
		From:           in.From,
		SendTo:         in.To,
		SendAs:         in.SendAs,
		Body:           in.Body,
		To:             to,
		Type:           model.MessageTypeSystem,
		IdempotencyKey: in.SendID,
		Metadata:       model.BuildMetadata(in.Body),
		System: &model.MessageSystem{
			Type:     botStoppedSystemType,
			Metadata: make(map[string]any),
		},
	}

	msg.SetMemberFromSlice(t.Members)

	return msg
}

func memberByContactID(members []*model.ThreadDialog, contactID uuid.UUID) *model.ThreadDialog {
	for _, m := range members {
		if m != nil && m.ContactID == contactID {
			return m
		}
	}

	return nil
}

func (s *MessageService) SendDocument(ctx context.Context, in *dto.SendDocumentRequest) (*dto.SendDocumentResponse, error) {
	log := s.logger.With("operation", "send_document")

	if err := in.Validate(); err != nil {
		log.Warn("send document request validation", "error", err)

		return nil, errors.InvalidArgument("send document request validation", errors.WithCause(err), errors.WithID("service.message.send_document"))
	}

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		From:     &in.From,
		To:       &in.To,
		DomainID: int(in.DomainID),
		SendAs:   in.SendAs,
		ToIsBot:  func() bool { return s.resolveToIsBot(ctx, in.To.ID, int(in.DomainID)) },
	})
	if err != nil {
		log.Error("resolving thread", "error", err, "from", in.From.ID.String(), "to", in.To.ID.String(), "to_type", in.To.Type.String())

		return nil, err
	}

	replyPreview, err := s.resolveReply(ctx, in.ReplyToMessageID, in.ReplyToExternalID, &in.From, t, int32(in.DomainID))
	if err != nil {
		return nil, err
	}

	attachments := make([]AttachmentProcessor, len(in.Document.Documents))
	for i, doc := range in.Document.Documents {
		attachments[i] = doc
	}

	if err := s.mediaProcessor.Process(ctx, in.DomainID, attachments); err != nil {
		log.Error("processing input media", "error", err)

		return nil, err
	}

	filesMetadata, err := s.mediaProcessor.FetchFileLinksWithMetadata(ctx, in.DomainID, attachments)
	if err != nil {
		return nil, errors.Wrap(err, errors.WithID("service.message.send_document"))
	}

	for i := range attachments {
		fileMetadata, ok := filesMetadata.FilesMetadata[attachments[i].GetID()]
		if !ok {
			continue
		}

		attachments[i].SetMime(fileMetadata.Mime)
		attachments[i].SetName(fileMetadata.Name)
		attachments[i].SetURL(fileMetadata.URL)
		attachments[i].SetSize(fileMetadata.Size)
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
	msg.BotControllerMemberID = t.BotControllerID
	msg.SendAs = in.SendAs
	msg.ReplyTo = replyPreview
	msg.SetMemberFromSlice(t.Members)

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		saved, err := uow.Messages().SaveMessage(txCtx, msg)
		if err != nil {
			return errors.Internal("save message", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		_, err = uow.Messages().SaveDocuments(txCtx, saved.ID, msg.Documents)
		if err != nil {
			return errors.Internal("save documents", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		msg.ID = saved.ID
		msg.From = saved.From

		if err = s.recordInboundExternalID(txCtx, uow, msg, &in.From, in.ExternalID); err != nil {
			return errors.Internal("save external message id", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		msg.WithCreatedEvent(ctx, in.SendID)

		if err := s.dispatchMessageEvents(txCtx, uow, msg); err != nil {
			return errors.Internal("dispatch message events", errors.WithCause(err), errors.WithID("service.message.send_document"))
		}

		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "send document failed", "err", err)

		return nil, err
	}

	if err = s.sendMessageToExternalProvider(ctx, msg); err != nil {
		log.Error("sending document message to external providers", "error", err)
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
		savedMsg.ReplyTo = msg.ReplyTo

		savedMsg.WithCreatedEvent(ctx, msg.IdempotencyKey)

		return s.dispatchMessageEvents(txCtx, uow, savedMsg)
	})
	if err != nil {
		log.ErrorContext(ctx, "location_transaction_failed", "err", err)

		return nil, err
	}

	if err = s.sendMessageToExternalProvider(ctx, msg); err != nil {
		log.Error("sending location message to external providers", "error", err)
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
		savedMsg.ReplyTo = msg.ReplyTo

		savedMsg.WithCreatedEvent(ctx, msg.IdempotencyKey)

		return s.dispatchMessageEvents(txCtx, uow, savedMsg)
	})
	if err != nil {
		log.ErrorContext(ctx, "contact_transaction_failed", "err", err)

		return nil, err
	}

	if err = s.sendMessageToExternalProvider(ctx, msg); err != nil {
		log.Error("sending contact message to external providers", "error", err)
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
		savedMsg.WithCreatedEvent(ctx, msg.IdempotencyKey)

		return s.dispatchMessageEvents(ctx, uow, savedMsg)
	})
	if err != nil {
		log.ErrorContext(ctx, "transaction failed", "err", err)

		return nil, err
	}

	if err = s.sendMessageToExternalProvider(ctx, msg); err != nil {
		log.Error("sending interactive message to external providers", "error", err)
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

		savedCallback.WithCreatedEvent(ctx)

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

		savedMsg.WithCreatedEvent(ctx, msg.IdempotencyKey)

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
		SendAs:   msg.SendAs,
		ToIsBot:  func() bool { return s.resolveToIsBot(ctx, msg.SendTo.ID, int(msg.DomainID)) },
	})
	if err != nil {
		return err
	}

	msg.ThreadID = t.ID
	msg.To = t.Members
	msg.BotControllerMemberID = t.BotControllerID

	msg.SetMemberFromSlice(t.Members)

	if msg.ReplyTo != nil {
		targetID := msg.ReplyTo.MessageID

		preview, err := s.resolveReplyPreview(ctx, &targetID, t.ID, msg.DomainID)
		if err != nil {
			return err
		}

		msg.ReplyTo = preview
	}

	return nil
}

func (s *MessageService) resolveReply(
	ctx context.Context,
	replyToID *uuid.UUID,
	replyToExternalID string,
	from *shared.Peer,
	t *model.Thread,
	domainID int32,
) (*model.ReplyToPreview, error) {
	if replyToID != nil {
		return s.resolveReplyPreview(ctx, replyToID, t.ID, domainID)
	}

	if replyToExternalID == "" {
		return nil, nil
	}

	gateID := gateIDForPeer(from, t.Members)
	if gateID == "" {
		return nil, nil
	}

	log := s.logger.With("gate_id", gateID, "reply_to_external_id", replyToExternalID)

	id, err := s.uow.MessageExternal().LookupMessageID(ctx, gateID, replyToExternalID)
	if err != nil {
		log.WarnContext(ctx, "external reply lookup failed; saving message without reply link", "err", err)

		return nil, nil
	}

	if id == uuid.Nil {
		log.WarnContext(ctx, "external reply reference not found; saving message without reply link")

		return nil, nil
	}

	preview, err := s.uow.Messages().GetReplyPreview(ctx, id, domainID)
	if err != nil || preview == nil || preview.ThreadID != t.ID {
		log.WarnContext(ctx, "external reply target unusable; saving message without reply link", "err", err)

		return nil, nil
	}

	return preview, nil
}

func (s *MessageService) resolveReplyPreview(ctx context.Context, replyToID *uuid.UUID, threadID uuid.UUID, domainID int32) (*model.ReplyToPreview, error) {
	if replyToID == nil {
		return nil, nil
	}

	if *replyToID == uuid.Nil {
		return nil, errors.InvalidArgument("reply_to_message_id is not a valid uuid", errors.WithID("service.message.reply_target"))
	}

	preview, err := s.uow.Messages().GetReplyPreview(ctx, *replyToID, domainID)
	if err != nil {
		return nil, err
	}

	if preview == nil || preview.ThreadID != threadID {
		return nil, errors.InvalidArgument("reply target message not found in this thread", errors.WithID("service.message.reply_target"))
	}

	return preview, nil
}

func (s *MessageService) recordInboundExternalID(ctx context.Context, uow store.UnitOfWork, msg *model.Message, from *shared.Peer, externalID string) error {
	if externalID == "" {
		return nil
	}

	gateID := gateIDForPeer(from, msg.To)
	if gateID == "" {
		s.logger.WarnContext(ctx, "inbound message has external id but no gate; skipping mapping",
			"message_id", msg.ID.String(),
			"external_id", externalID,
		)

		return nil
	}

	return uow.MessageExternal().Save(ctx, &model.MessageExternalID{
		MessageID:  msg.ID,
		ThreadID:   msg.ThreadID,
		GateID:     gateID,
		ExternalID: externalID,
		Direction:  model.ExternalDirectionInbound,
	})
}

func gateIDForPeer(peer *shared.Peer, members []*model.ThreadDialog) string {
	if peer == nil {
		return ""
	}

	if peer.Identity != nil && peer.Identity.Via != nil && *peer.Identity.Via != "" {
		return *peer.Identity.Via
	}

	for _, m := range members {
		if m != nil && m.ContactID == peer.ID && m.Via != nil && *m.Via != "" {
			return *m.Via
		}
	}

	return ""
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
		return errors.New("domain events queue is empty: transaction aborted")
	}

	return s.dispatchEvents(ctx, uow, evs, func(e event.Outboxer) string {
		return fmt.Sprintf("im_message.%s.message.%s.%s",
			e.RecipientID(),
			"created",
			e.Version(),
		)
	})
}

func buildMessageCreatedTopic(recipientID uuid.UUID, version string) string {
	return fmt.Sprintf("im_message.%s.message.created.%s", recipientID, version)
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
