package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

type ThreadTagService struct {
	uow    store.UnitOfWork
	logger *slog.Logger
}

func NewThreadTagService(uow store.UnitOfWork, logger *slog.Logger) *ThreadTagService {
	if logger == nil {
		logger = slog.Default()
	}

	return &ThreadTagService{
		uow:    uow,
		logger: logger.With(slog.String("component", "thread_tag_service")),
	}
}

func (s *ThreadTagService) AddTag(ctx context.Context, req *dto.AddTagRequest) (*model.ThreadTag, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_tag.add_tag"))
	}

	if req.ContactID == uuid.Nil {
		return nil, errors.InvalidArgument("contact_id is required", errors.WithID("service.thread_tag.add_tag"))
	}

	if req.ThreadID == uuid.Nil {
		return nil, errors.InvalidArgument("thread_id is required", errors.WithID("service.thread_tag.add_tag"))
	}

	if req.Tag == "" {
		return nil, errors.InvalidArgument("tag is required", errors.WithID("service.thread_tag.add_tag"))
	}

	var result *model.ThreadTag

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		member, ok, err := activeMember(ctx, uow, req.ThreadID, req.ContactID)
		if err != nil {
			return err
		}

		if !ok {
			return errors.Forbidden("only thread members can tag the thread", errors.WithID("service.thread_tag.add_tag"))
		}

		var inserted bool

		result, inserted, err = uow.ThreadTagStore().Add(ctx, &model.ThreadTag{
			ThreadID:  req.ThreadID,
			ContactID: req.ContactID,
			Tag:       req.Tag,
		})
		if err != nil || !inserted {
			return err
		}

		return publishTagsChanged(ctx, uow, req.ThreadID, req.ContactID, int32(member.DomainID))
	})
	if err != nil {
		s.logger.Error("adding thread tag", "operation", "service.thread_tag.add_tag", "err", err)

		return nil, err
	}

	return result, nil
}

func (s *ThreadTagService) RemoveTag(ctx context.Context, req *dto.RemoveTagRequest) error {
	if req == nil {
		return errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_tag.remove_tag"))
	}

	if req.TagID == uuid.Nil {
		return errors.InvalidArgument("tag_id is required", errors.WithID("service.thread_tag.remove_tag"))
	}

	if req.ContactID == uuid.Nil {
		return errors.InvalidArgument("contact_id is required", errors.WithID("service.thread_tag.remove_tag"))
	}

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		threadID, err := uow.ThreadTagStore().Remove(ctx, req.TagID, req.ContactID)
		if err != nil {
			return err
		}

		return notifyMemberTagsChanged(ctx, uow, threadID, req.ContactID)
	})
	if err != nil {
		s.logger.Error("removing thread tag", "operation", "service.thread_tag.remove_tag", "err", err)

		return err
	}

	return nil
}

func (s *ThreadTagService) RemoveTagByName(ctx context.Context, req *dto.RemoveTagByNameRequest) error {
	if req == nil {
		return errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_tag.remove_tag_by_name"))
	}

	if req.ContactID == uuid.Nil {
		return errors.InvalidArgument("contact_id is required", errors.WithID("service.thread_tag.remove_tag_by_name"))
	}

	if req.ThreadID == uuid.Nil {
		return errors.InvalidArgument("thread_id is required", errors.WithID("service.thread_tag.remove_tag_by_name"))
	}

	if req.Tag == "" {
		return errors.InvalidArgument("tag is required", errors.WithID("service.thread_tag.remove_tag_by_name"))
	}

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		removed, err := uow.ThreadTagStore().RemoveByName(ctx, req.ThreadID, req.ContactID, req.Tag)
		if err != nil || !removed {
			return err
		}

		return notifyMemberTagsChanged(ctx, uow, req.ThreadID, req.ContactID)
	})
	if err != nil {
		s.logger.Error("removing thread tag by name", "operation", "service.thread_tag.remove_tag_by_name", "err", err)

		return err
	}

	return nil
}

func (s *ThreadTagService) SearchTags(ctx context.Context, req *dto.SearchTagsRequest) ([]string, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_tag.search_tags"))
	}

	if req.ContactID == uuid.Nil {
		return nil, errors.InvalidArgument("contact_id is required", errors.WithID("service.thread_tag.search_tags"))
	}

	tags, err := s.uow.ThreadTagStore().SearchTags(ctx, req.ContactID, req.Page, req.Size)
	if err != nil {
		s.logger.Error("searching thread tags", "operation", "service.thread_tag.search_tags", "err", err)

		return nil, err
	}

	return tags, nil
}

func (s *ThreadTagService) ListTags(ctx context.Context, threadID, contactID uuid.UUID) ([]string, error) {
	return s.uow.ThreadTagStore().ListNames(ctx, threadID, contactID)
}

const SnoozedTag = "snoozed"

var incomingClearsTags = []string{SnoozedTag}

func (s *ThreadTagService) Snooze(ctx context.Context, threadID, contactID uuid.UUID) (*model.ThreadTag, error) {
	return s.AddTag(ctx, &dto.AddTagRequest{ContactID: contactID, ThreadID: threadID, Tag: SnoozedTag})
}

func (s *ThreadTagService) Unsnooze(ctx context.Context, threadID, contactID uuid.UUID) error {
	return s.RemoveTagByName(ctx, &dto.RemoveTagByNameRequest{ContactID: contactID, ThreadID: threadID, Tag: SnoozedTag})
}

func (s *ThreadTagService) OnMessage(ctx context.Context, uow store.UnitOfWork, msg *model.Message) error {
	recipients := msg.RecipientIDs()
	if len(recipients) == 0 {
		return nil
	}

	cleared, err := uow.ThreadTagStore().RemoveForContacts(ctx, msg.ThreadID, recipients, incomingClearsTags)
	if err != nil {
		return err
	}

	for _, contactID := range cleared {
		if err := publishTagsChanged(ctx, uow, msg.ThreadID, contactID, msg.DomainID); err != nil {
			return err
		}
	}

	return nil
}

func (s *ThreadTagService) OnMemberLeft(ctx context.Context, uow store.UnitOfWork, threadID, contactID uuid.UUID) error {
	_, err := uow.ThreadTagStore().RemoveForContacts(ctx, threadID, []uuid.UUID{contactID}, nil)

	return err
}

func activeMember(ctx context.Context, uow store.UnitOfWork, threadID, contactID uuid.UUID) (*model.ThreadDialog, bool, error) {
	members, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
		ThreadIDs:  []uuid.UUID{threadID},
		ContactIDs: []uuid.UUID{contactID},
		Limit:      1,
	})
	if err != nil || len(members) == 0 {
		return nil, false, err
	}

	return members[0], true, nil
}

func notifyMemberTagsChanged(ctx context.Context, uow store.UnitOfWork, threadID, contactID uuid.UUID) error {
	member, ok, err := activeMember(ctx, uow, threadID, contactID)
	if err != nil || !ok {
		return err
	}

	return publishTagsChanged(ctx, uow, threadID, contactID, int32(member.DomainID))
}

func publishTagsChanged(ctx context.Context, uow store.UnitOfWork, threadID, contactID uuid.UUID, domainID int32) error {
	tags, err := uow.ThreadTagStore().ListNames(ctx, threadID, contactID)
	if err != nil {
		return err
	}

	e := event.NewThreadTagsChanged(threadID, contactID, domainID, tags)

	return uow.Outbox().Publish(ctx, e.Topic(), e)
}
