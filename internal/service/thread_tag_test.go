package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type stubThreadTagStore struct {
	addErr error
}

func (s *stubThreadTagStore) Add(ctx context.Context, tag *model.ThreadTag) (*model.ThreadTag, error) {
	if s.addErr != nil {
		return nil, s.addErr
	}

	if tag == nil {
		return nil, errors.InvalidArgument("tag cannot be nil")
	}

	tag.ID = uuid.New()

	return tag, nil
}

func (s *stubThreadTagStore) Remove(ctx context.Context, tagID, contactID uuid.UUID) error {
	return nil
}

func (s *stubThreadTagStore) ListForContact(ctx context.Context, contactID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID][]*model.ThreadTag, error) {
	return make(map[uuid.UUID][]*model.ThreadTag), nil
}

func (s *stubThreadTagStore) SearchTags(ctx context.Context, contactID uuid.UUID, page, size int) ([]string, error) {
	return nil, nil
}

func TestThreadTagServiceAddTag(t *testing.T) {
	tagStore := &stubThreadTagStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore}
	svc := NewThreadTagService(uow, nil)

	req := &dto.AddTagRequest{
		ContactID: uuid.New(),
		ThreadID:  uuid.New(),
		Tag:       "important",
	}

	result, err := svc.AddTag(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, req.Tag, result.Tag)
}

func TestThreadTagServiceAddTagValidation(t *testing.T) {
	tagStore := &stubThreadTagStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore}
	svc := NewThreadTagService(uow, nil)

	_, err := svc.AddTag(context.Background(), nil)
	require.Error(t, err)

	req := &dto.AddTagRequest{
		ContactID: uuid.New(),
		ThreadID:  uuid.New(),
		Tag:       "",
	}
	_, err = svc.AddTag(context.Background(), req)
	require.Error(t, err)
}

func TestThreadTagServiceRemoveTag(t *testing.T) {
	tagStore := &stubThreadTagStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore}
	svc := NewThreadTagService(uow, nil)

	req := &dto.RemoveTagRequest{
		TagID:     uuid.New(),
		ContactID: uuid.New(),
	}

	err := svc.RemoveTag(context.Background(), req)
	require.NoError(t, err)
}

func TestThreadTagServiceSearchTags(t *testing.T) {
	tagStore := &stubThreadTagStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore}
	svc := NewThreadTagService(uow, nil)

	req := &dto.SearchTagsRequest{
		ContactID: uuid.New(),
		Page:      1,
		Size:      20,
	}

	result, err := svc.SearchTags(context.Background(), req)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestThreadTagServiceSearchTagsValidation(t *testing.T) {
	tagStore := &stubThreadTagStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore}
	svc := NewThreadTagService(uow, nil)

	_, err := svc.SearchTags(context.Background(), nil)
	require.Error(t, err)

	req := &dto.SearchTagsRequest{
		ContactID: uuid.Nil,
		Page:      1,
		Size:      20,
	}
	_, err = svc.SearchTags(context.Background(), req)
	require.Error(t, err)
}
