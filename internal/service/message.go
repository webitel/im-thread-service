package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/config"
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

	// Ephemeral typing-indicator collaborators (see typing.go).
	typingBus   TypingBus
	rateLimiter RateLimiter
	typingCfg   config.TypingConfig
}

func NewMessageService(
	uow store.UnitOfWork,
	logger *slog.Logger,
	threader ThreadManager,
	contactClient *imcontact.Client,
	mediaProcessor MediaProcessor,
	providersAdapter ProvidersAdapter,
	typingBus TypingBus,
	rateLimiter RateLimiter,
	typingCfg config.TypingConfig,
) *MessageService {
	return &MessageService{
		uow:              uow,
		logger:           logger,
		threader:         threader,
		contactClient:    contactClient,
		mediaProcessor:   mediaProcessor,
		providersAdapter: providersAdapter,
		typingBus:        typingBus,
		rateLimiter:      rateLimiter,
		typingCfg:        typingCfg,
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
		ForwardOrigin:         in.ForwardOrigin,
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

		if err = s.insertSentStatuses(ctx, uow, saved); err != nil {
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

		if e = s.insertSentStatuses(txCtx, uow, saved); e != nil {
			return e
		}

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
	msg.ForwardOrigin = in.ForwardOrigin
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

		if err := s.insertSentStatuses(txCtx, uow, msg); err != nil {
			return errors.Internal("insert sent statuses", errors.WithCause(err), errors.WithID("service.message.send_document"))
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

// Read marks the thread as read by the user up to the given message
// (inclusive): every earlier unread message of the recipient is covered by
// a single bulk update and a single status event.
func (s *MessageService) Read(ctx context.Context, in *dto.ReadMessageRequest) error {
	if err := guards.ValidateReadMessage(in); err != nil {
		return fmt.Errorf("validation: %w", err)
	}

	tID, _ := uuid.Parse(in.ThreadID)
	mID, _ := uuid.Parse(in.MessageID)
	uID, _ := uuid.Parse(in.UserID)

	return s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		changes, err := uow.MessageStatuses().MarkRead(txCtx, []*model.ReadReceipt{{
			DomainID:      in.DomainID,
			ThreadID:      tID,
			MemberID:      uID,
			UpToMessageID: mID,
		}})
		if err != nil {
			return fmt.Errorf("read_message: %w", err)
		}

		return dispatchStatusChangeEvents(txCtx, uow, changes)
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

		if err = s.insertSentStatuses(txCtx, uow, savedMsg); err != nil {
			return err
		}

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

		if err = s.insertSentStatuses(txCtx, uow, savedMsg); err != nil {
			return err
		}

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

		if err = s.insertSentStatuses(ctx, uow, savedMsg); err != nil {
			return err
		}

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

		if err = s.insertSentStatuses(txCtx, uow, savedMsg); err != nil {
			return err
		}

		savedMsg.WithCreatedEvent(ctx, msg.IdempotencyKey)

		return s.dispatchMessageEvents(txCtx, uow, savedMsg)
	})
	if err != nil {
		log.ErrorContext(ctx, "transaction_failed", "err", err)

		return nil, err
	}

	return savedMsg, nil
}

func (s *MessageService) EditMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	log := s.logger.With("operation", "edit_message")

	if msg == nil || msg.ID == uuid.Nil {
		return nil, errors.InvalidArgument("message id is required", errors.WithID("service.message.edit_message"))
	}

	editor := msg.From
	if editor.ID == uuid.Nil {
		return nil, errors.InvalidArgument("editor identity is required", errors.WithID("service.message.edit_message"))
	}

	var editedMsg *model.Message

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var err error
		if editedMsg, err = uow.Messages().EditMessage(txCtx, msg); err != nil {
			return err
		}

		members, err := uow.ThreadDialogStore().GetQuickView(txCtx, &model.ThreadDialogStoreFilter{
			ThreadIDs:      []uuid.UUID{editedMsg.ThreadID},
			IncludeDeleted: false,
		})
		if err != nil {
			return err
		}

		editedMsg.To = members
		editedMsg.From = editor
		editedMsg.WithEditedEvent(txCtx)

		return s.dispatchMessageEvents(txCtx, uow, editedMsg)
	})
	if err != nil {
		log.ErrorContext(ctx, "transaction_failed", "err", err)

		return nil, err
	}

	return editedMsg, nil
}

// DeleteMessages soft-deletes the caller's own messages, provided the caller
// still holds can_delete_messages in that thread. It is best-effort: messages
// the caller may not remove are reported as skipped rather than failing the
// whole batch, and only an empty result is an error. It is also idempotent, so
// a message an earlier attempt already deleted still counts as satisfied.
func (s *MessageService) DeleteMessages(ctx context.Context, in *dto.DeleteMessagesRequest) (*dto.DeleteMessagesResponse, error) {
	log := s.logger.With("operation", "delete_messages")

	if in == nil || len(in.IDs) == 0 {
		return nil, errors.InvalidArgument("message ids are required", errors.WithID("service.message.delete_messages"))
	}

	deleter := in.DeletedBy
	if deleter.ID == uuid.Nil {
		return nil, errors.InvalidArgument("deleter identity is required", errors.WithID("service.message.delete_messages"))
	}

	var deleted []*model.Message

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var err error
		if deleted, err = uow.Messages().DeleteMessages(txCtx, in.IDs, deleter.ID); err != nil {
			return err
		}

		if len(deleted) == 0 {
			return errors.Forbidden(
				"no messages could be deleted: not found, not authored by the caller, the chat is closed, or the caller may not delete messages there",
				errors.WithID("service.message.delete.not_allowed"),
			)
		}

		// A batch may span several threads, so resolve the recipient list once
		// per thread rather than once per message.
		membersByThread := make(map[uuid.UUID][]*model.ThreadDialog)

		for _, msg := range deleted {
			// An already-deleted message sent its event on the first call.
			if !msg.JustDeleted {
				continue
			}

			members, ok := membersByThread[msg.ThreadID]
			if !ok {
				if members, err = uow.ThreadDialogStore().GetQuickView(txCtx, &model.ThreadDialogStoreFilter{
					ThreadIDs:      []uuid.UUID{msg.ThreadID},
					IncludeDeleted: false,
				}); err != nil {
					return err
				}

				membersByThread[msg.ThreadID] = members
			}

			msg.To = members
			msg.From = deleter
			msg.WithDeletedEvent(txCtx)

			if err = s.dispatchMessageEvents(txCtx, uow, msg); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		log.ErrorContext(ctx, "transaction_failed", "err", err)

		return nil, err
	}

	return buildDeleteMessagesResponse(in.IDs, deleted), nil
}

// SetReaction sets, replaces or clears the caller's single emoji reaction on a
// message. It is idempotent per send_id: every at-least-once redelivery of the
// same request is a no-op (dedup ledger), so a retried toggle-off never
// resurrects a reaction and a reordered retry never reverts a newer state.
// Distinct requests apply in arrival order (last-writer-wins). A no-op change
// publishes nothing; a real change is fanned out to both sides via a
// MessageReaction event and forwarded best-effort to the external messenger
// (subject to the messenger's capabilities).
func (s *MessageService) SetReaction(ctx context.Context, in *dto.SetReactionRequest) (*dto.SetReactionResponse, error) {
	log := s.logger.With("operation", "set_reaction")

	if err := guards.ValidateSetReaction(in); err != nil {
		return nil, err
	}

	reaction := &model.Reaction{
		MessageID:      in.MessageID,
		DomainID:       in.DomainID,
		ReactorID:      in.Reactor.ID,
		Emoji:          in.Emoji,
		IdempotencyKey: in.IdempotencyKey,
		ExternalID:     in.ExternalID,
		Reactor:        in.Reactor,
	}

	if in.ThreadID != nil {
		reaction.ThreadID = *in.ThreadID
	}

	var result *model.ReactionResult

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		var err error
		if result, err = uow.MessageReactions().SetReaction(txCtx, reaction); err != nil {
			if errors.Is(err, store.ErrReactionNotAllowed) {
				return errors.Forbidden(
					"reaction not allowed: message not found, deleted, or the reactor may not react in this thread",
					errors.WithID("service.message.set_reaction.not_allowed"),
				)
			}

			return err
		}

		if !result.Changed {
			return nil
		}

		reaction.ThreadID = result.ThreadID

		members, err := uow.ThreadDialogStore().GetQuickView(txCtx, &model.ThreadDialogStoreFilter{
			ThreadIDs:      []uuid.UUID{reaction.ThreadID},
			IncludeDeleted: false,
		})
		if err != nil {
			return err
		}

		reaction.To = members
		reaction.WithReactionEvent(txCtx, result)

		return s.dispatchReactionEvents(txCtx, uow, reaction)
	})
	if err != nil {
		log.ErrorContext(ctx, "transaction_failed", "err", err)

		return nil, err
	}

	if result.Changed {
		if err = s.providersAdapter.SendReaction(ctx, reaction, result); err != nil {
			log.Error("forwarding reaction to external providers", "error", err)
		}
	}

	return &dto.SetReactionResponse{
		Action:    result.Action,
		Emoji:     result.Emoji,
		ReactedAt: result.ReactedAt,
	}, nil
}

func buildDeleteMessagesResponse(requested uuid.UUIDs, deleted []*model.Message) *dto.DeleteMessagesResponse {
	deletedIDs := make(uuid.UUIDs, 0, len(deleted))
	deletedSet := make(map[uuid.UUID]struct{}, len(deleted))

	for _, msg := range deleted {
		deletedIDs = append(deletedIDs, msg.ID)
		deletedSet[msg.ID] = struct{}{}
	}

	skippedIDs := make(uuid.UUIDs, 0, len(requested)-len(deleted))

	for _, id := range requested {
		if _, ok := deletedSet[id]; !ok {
			skippedIDs = append(skippedIDs, id)
		}
	}

	return &dto.DeleteMessagesResponse{
		DeletedIDs: deletedIDs,
		SkippedIDs: skippedIDs,
		DeletedAt:  deleted[0].DeletedAtOrNow(),
	}
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
		preview, err := s.resolveReplyPreview(ctx, msg.ReplyTo.MessageID, t.ID, msg.DomainID)
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
		return s.resolveReplyPreview(ctx, *replyToID, t.ID, domainID)
	}

	return s.resolveExternalReply(ctx, replyToExternalID, from, t, domainID), nil
}

func (s *MessageService) resolveExternalReply(
	ctx context.Context,
	replyToExternalID string,
	from *shared.Peer,
	t *model.Thread,
	domainID int32,
) *model.ReplyToPreview {
	if replyToExternalID == "" {
		return nil
	}

	gateID := gateIDForPeer(from, t.Members)
	if gateID == "" {
		return nil
	}

	log := s.logger.With("gate_id", gateID, "reply_to_external_id", replyToExternalID)

	id, err := s.uow.MessageExternal().LookupMessageID(ctx, gateID, replyToExternalID)
	if err != nil {
		log.WarnContext(ctx, "external reply lookup failed; saving message without reply link", "err", err)

		return nil
	}

	if id == uuid.Nil {
		log.WarnContext(ctx, "external reply reference not found; saving message without reply link")

		return nil
	}

	preview, err := s.uow.Messages().GetReplyPreview(ctx, id, domainID)
	if err != nil || preview.ThreadID != t.ID {
		log.WarnContext(ctx, "external reply target unusable; saving message without reply link", "err", err)

		return nil
	}

	return preview
}

func (s *MessageService) resolveReplyPreview(ctx context.Context, replyToID, threadID uuid.UUID, domainID int32) (*model.ReplyToPreview, error) {
	if replyToID == uuid.Nil {
		return nil, errors.InvalidArgument("reply_to_message_id is not a valid uuid", errors.WithID("service.message.reply_target"))
	}

	preview, err := s.uow.Messages().GetReplyPreview(ctx, replyToID, domainID)
	if err != nil {
		if errors.Is(err, store.ErrReplyTargetNotFound) {
			return nil, errors.InvalidArgument("reply target message not found in this thread", errors.WithID("service.message.reply_target"))
		}

		return nil, err
	}

	if preview.ThreadID != threadID {
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

// statusViaBot is the confirmation source recorded for bot recipients.
const statusViaBot = "bot"

// insertSentStatuses creates per-recipient SENT status rows for a freshly
// saved message: every thread member except the effective sender. No status
// event is published — MessageCreated already implies the initial SENT state.
//
// Bot members are promoted to DELIVERED right away: the transactional
// outbox hand-off is the delivery into the bot pipeline, and bots publish
// no receipts of their own, so DELIVERED is their terminal state. The
// promotion does publish a status event so timelines update live.
func (s *MessageService) insertSentStatuses(ctx context.Context, uow store.UnitOfWork, msg *model.Message) error {
	sender := msg.GetSender()

	recipients := make([]uuid.UUID, 0, len(msg.To))

	var botReceipts []*model.StatusReceipt

	for _, member := range msg.To {
		if member == nil || member.ContactID == sender {
			continue
		}

		recipients = append(recipients, member.ContactID)

		if member.IsBot {
			botReceipts = append(botReceipts, &model.StatusReceipt{
				DomainID:  msg.DomainID,
				ThreadID:  msg.ThreadID,
				MessageID: msg.ID,
				MemberID:  member.ContactID,
				Via:       statusViaBot,
			})
		}
	}

	if err := uow.MessageStatuses().InsertSent(ctx, msg, recipients); err != nil {
		return err
	}

	if len(botReceipts) == 0 {
		return nil
	}

	changes, err := uow.MessageStatuses().MarkDelivered(ctx, botReceipts)
	if err != nil {
		return fmt.Errorf("marking bot recipients delivered: %w", err)
	}

	return dispatchStatusChangeEvents(ctx, uow, changes)
}

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

func (s *MessageService) dispatchReactionEvents(ctx context.Context, uow store.UnitOfWork, reaction *model.Reaction) error {
	evs := reaction.Events()
	if len(evs) == 0 {
		return nil
	}

	return s.dispatchEvents(ctx, uow, evs, func(e event.Outboxer) string {
		return fmt.Sprintf("im_message.%s.message.reaction.%s",
			e.RecipientID(),
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
			messageEventAction(e),
			e.Version(),
		)
	})
}

func messageEventAction(e event.Outboxer) string {
	switch e.EventType() {
	case event.MessageEditedEvent:
		return "edited"
	case event.MessageDeletedEvent:
		return "deleted"
	case event.MessageReactionEvent:
		return "reaction"
	default:
		return "created"
	}
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
