package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	contactv1 "github.com/webitel/im-thread-service/gen/go/contact/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
)

// ForwardMessages copies messages the caller can read into a direct thread with
// the target, stamping each copy with a snapshot of its original author. It is
// best-effort: sources the caller cannot read, that no longer exist, or that
// cannot be forwarded come back as skipped rather than failing the whole batch.
func (s *MessageService) ForwardMessages(ctx context.Context, in *dto.ForwardMessagesRequest) (*dto.ForwardMessagesResponse, error) {
	if err := guards.ForwardMessagesGuard(in); err != nil {
		return nil, errors.InvalidArgument(
			"validating forward request",
			errors.WithCause(err),
			errors.WithID("service.message.forward"),
		)
	}

	log := s.logger.With(
		"operation", "message.ForwardMessages",
		slog.String("from", in.From.ID.String()),
		slog.String("to", in.To.ID.String()),
		slog.Int("requested", len(in.MessageIDs)),
	)

	sources, err := s.uow.Messages().LoadForwardSources(ctx, in.MessageIDs, in.From.ID, int32(in.DomainID))
	if err != nil {
		return nil, err
	}

	if len(sources) == 0 {
		return nil, errors.NotFound(
			"none of the requested messages can be forwarded",
			errors.WithID("service.message.forward.no_sources"),
		)
	}

	originNames := s.resolveOriginNames(ctx, sources, int32(in.DomainID))

	t, err := s.threader.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{
		From:     &in.From,
		To:       &in.To,
		DomainID: int(in.DomainID),
		SendAs:   in.SendAs,
		ToIsBot:  func() bool { return s.resolveToIsBot(ctx, in.To.ID, int(in.DomainID)) },
	})
	if err != nil {
		log.ErrorContext(ctx, "failed to ensure destination thread", "err", err)

		return nil, err
	}

	copies := make([]*model.Message, 0, len(sources))

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		copies = copies[:0]

		for _, src := range sources {
			copied, err := s.forwardOne(txCtx, uow, in, t, src, originNames[src.SenderID])
			if err != nil {
				return err
			}

			copies = append(copies, copied)
		}

		return nil
	})
	if err != nil {
		log.ErrorContext(ctx, "forward transaction failed", "err", err, slog.String("thread_id", t.ID.String()))

		return nil, err
	}

	for _, copied := range copies {
		if err := s.sendMessageToExternalProvider(ctx, copied); err != nil {
			log.Error("sending forwarded message to external providers", "error", err,
				slog.String("message_id", copied.ID.String()))
		}
	}

	return buildForwardResponse(in, t.ID, copies), nil
}

func (s *MessageService) forwardOne(
	ctx context.Context,
	uow store.UnitOfWork,
	in *dto.ForwardMessagesRequest,
	t *model.Thread,
	src *model.Message,
	originName string,
) (*model.Message, error) {
	msg := &model.Message{
		ThreadID:              t.ID,
		DomainID:              int32(in.DomainID),
		From:                  in.From,
		To:                    t.Members,
		Body:                  src.Body,
		Type:                  forwardedType(src),
		Metadata:              model.BuildMetadata(src.Body),
		SendAs:                in.SendAs,
		BotControllerMemberID: t.BotControllerID,
		ForwardOrigin:         model.NewInternalForwardOrigin(src, originName),
		Location:              src.Location,
		Contact:               src.Contact,
		Documents:             src.Documents,
		Images:                src.Images,
	}

	msg.SetMemberFromSlice(t.Members)

	saved, err := s.saveForwardedMessage(ctx, uow, msg)
	if err != nil {
		return nil, err
	}

	saved.To = t.Members
	saved.From = in.From
	saved.SendAs = in.SendAs
	saved.ForwardOrigin = msg.ForwardOrigin
	saved.BotControllerMemberID = t.BotControllerID
	saved.Documents = msg.Documents
	saved.Images = msg.Images
	saved.Location = msg.Location
	saved.Contact = msg.Contact
	saved.Member = msg.Member

	if len(src.Documents) > 0 || len(src.Images) > 0 {
		if err := uow.Messages().CopyAttachments(ctx, src.ID, saved.ID); err != nil {
			return nil, err
		}
	}

	if err := s.insertSentStatuses(ctx, uow, saved); err != nil {
		return nil, err
	}

	saved.WithCreatedEvent(ctx, in.SendID)

	if err := s.dispatchMessageEvents(ctx, uow, saved); err != nil {
		return nil, err
	}

	return saved, nil
}

func (s *MessageService) saveForwardedMessage(
	ctx context.Context,
	uow store.UnitOfWork,
	msg *model.Message,
) (*model.Message, error) {
	switch {
	case msg.Type == model.MessageTypeLocation && msg.Location != nil:
		return uow.Messages().SaveMessageLocation(ctx, msg)
	case msg.Type == model.MessageTypeContact && msg.Contact != nil:
		return uow.Messages().SaveMessageContact(ctx, msg)
	default:
		return uow.Messages().SaveMessage(ctx, msg)
	}
}

// forwardedType maps a source message onto the type its copy should carry.
// Interactive messages degrade to text: their buttons are bound to
// buttons_callback rows in the source thread and would be dead at the
// destination.
func forwardedType(src *model.Message) model.MessageType {
	switch src.Type {
	case model.MessageTypeInteractive:
		if len(src.Documents) > 0 || len(src.Images) > 0 {
			return model.MessageTypeFile
		}

		return model.MessageTypeText
	case model.MessageTypeLocation:
		if src.Location == nil {
			return model.MessageTypeText
		}
	case model.MessageTypeContact:
		if src.Contact == nil {
			return model.MessageTypeText
		}
	}

	return src.Type
}

func (s *MessageService) resolveOriginNames(
	ctx context.Context,
	sources []*model.Message,
	domainID int32,
) map[uuid.UUID]string {
	names := make(map[uuid.UUID]string, len(sources))

	if s.contactClient == nil {
		return names
	}

	ids := make([]string, 0, len(sources))

	for _, src := range sources {
		if src.SenderID == uuid.Nil {
			continue
		}

		if _, seen := names[src.SenderID]; seen {
			continue
		}

		names[src.SenderID] = ""
		ids = append(ids, src.SenderID.String())
	}

	if len(ids) == 0 {
		return names
	}

	res, err := s.contactClient.SearchContact(ctx, &contactv1.SearchContactRequest{
		Ids:      ids,
		DomainId: domainID,
		Size:     int32(len(ids)),
		Page:     1,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "resolving forward origin names failed; forwarding without labels", "err", err)

		return names
	}

	for _, contact := range res.GetContacts() {
		id, err := uuid.Parse(contact.GetId())
		if err != nil {
			continue
		}

		if _, wanted := names[id]; !wanted {
			continue
		}

		name := contact.GetName()
		if name == "" {
			name = contact.GetUsername()
		}

		names[id] = name
	}

	return names
}

func buildForwardResponse(
	in *dto.ForwardMessagesRequest,
	threadID uuid.UUID,
	copies []*model.Message,
) *dto.ForwardMessagesResponse {
	ids := make([]uuid.UUID, 0, len(copies))
	forwarded := make(map[uuid.UUID]struct{}, len(copies))

	for _, copied := range copies {
		ids = append(ids, copied.ID)

		if origin := copied.ForwardOrigin; origin != nil && origin.SourceMessageID != nil {
			forwarded[*origin.SourceMessageID] = struct{}{}
		}
	}

	skipped := make([]uuid.UUID, 0, len(in.MessageIDs)-len(copies))

	for _, id := range in.MessageIDs {
		if _, ok := forwarded[id]; !ok {
			skipped = append(skipped, id)
		}
	}

	return &dto.ForwardMessagesResponse{
		To:         in.To,
		ThreadID:   threadID,
		IDs:        ids,
		SkippedIDs: skipped,
	}
}
