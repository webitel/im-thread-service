package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type fakeUnitOfWork struct {
	threadDialogStore store.ThreadDialogStore
}

func (f fakeUnitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context, store.UnitOfWork) error) error {
	return fn(ctx, f)
}

func (f fakeUnitOfWork) ThreadDialogStore() store.ThreadDialogStore {
	return f.threadDialogStore
}

func (f fakeUnitOfWork) ThreadStore() store.ThreadStore {
	return nil
}

func (f fakeUnitOfWork) ThreadPermissionStore() store.ThreadPermissionStore {
	return nil
}

func (f fakeUnitOfWork) MessageHistory() store.MessageHistory {
	return nil
}

func (f fakeUnitOfWork) Messages() store.MessageStore {
	return nil
}

func (f fakeUnitOfWork) Outbox() store.OutboxStore {
	return nil
}

func (f fakeUnitOfWork) InteractiveCallback() store.InteractiveCallback {
	return nil
}

type fakeThreadDialogStore struct {
	fullViewResult []*model.ThreadDialogExtended
	lastFilter     *model.ThreadDialogStoreFilter
}

func (f *fakeThreadDialogStore) Create(ctx context.Context, threadDialog *model.ThreadDialogExtended) (*model.ThreadDialogExtended, error) {
	return nil, nil
}

func (f *fakeThreadDialogStore) Delete(ctx context.Context, memberID uuid.UUID) error {
	return nil
}

func (f *fakeThreadDialogStore) GetQuickView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error) {
	return nil, nil
}

func (f *fakeThreadDialogStore) GetFullView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialogExtended, error) {
	f.lastFilter = filter
	return f.fullViewResult, nil
}

func (f *fakeThreadDialogStore) FindActorsPair(ctx context.Context, initiatorsContact, targetMember uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error) {
	return nil, nil, nil
}

type fakeThreadStore struct{}

func (f fakeThreadStore) Create(ctx context.Context, req *model.Thread) (*model.Thread, error) {
	return nil, nil
}

func (f fakeThreadStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	return nil, nil
}

func (f fakeThreadStore) ResolveDirect(ctx context.Context, from, to uuid.UUID) (*model.Thread, error) {
	return nil, nil
}

type fakeOutboxStore struct{}

func (f fakeOutboxStore) Publish(ctx context.Context, topic string, event event.Outboxer) error {
	return nil
}

func (f fakeOutboxStore) Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error) {
	return 0, nil
}

func TestFindAddMemberActors_ResolvesInitiatorAndTargetByContactID(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	targetContactID := uuid.New()
	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{ContactID: initiatorContactID, ThreadID: threadID},
			{ContactID: targetContactID, ThreadID: threadID},
		},
	}
	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{threadDialogStore: threadDialogStore},
	}

	initiator, target, err := svc.findAddMemberActors(context.Background(), threadID, initiatorContactID, targetContactID)

	require.NoError(t, err)
	require.NotNil(t, initiator)
	require.NotNil(t, target)
	require.Equal(t, initiatorContactID, initiator.ContactID)
	require.Equal(t, targetContactID, target.ContactID)
	require.NotNil(t, threadDialogStore.lastFilter)
	require.Equal(t, []uuid.UUID{threadID}, threadDialogStore.lastFilter.ThreadIDs)
	require.Equal(t, []uuid.UUID{initiatorContactID, targetContactID}, threadDialogStore.lastFilter.ContactIDs)
}

var _ store.UnitOfWork = fakeUnitOfWork{}
var _ store.ThreadDialogStore = (*fakeThreadDialogStore)(nil)
var _ store.ThreadStore = nil
var _ store.OutboxStore = fakeOutboxStore{}
var _ = dto.AddMemberRequest{}
