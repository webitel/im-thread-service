package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/webitel/webitel-go-kit/pkg/errors"

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

	tag := &model.ThreadTag{
		ThreadID:  req.ThreadID,
		ContactID: req.ContactID,
		Tag:       req.Tag,
	}

	result, err := s.uow.ThreadTagStore().Add(ctx, tag)
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

	err := s.uow.ThreadTagStore().Remove(ctx, req.TagID, req.ContactID)
	if err != nil {
		s.logger.Error("removing thread tag", "operation", "service.thread_tag.remove_tag", "err", err)

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
