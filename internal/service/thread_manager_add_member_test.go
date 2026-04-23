package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
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
	initiatorPair  *model.ThreadDialogExtended
	targetPair     *model.ThreadDialogExtended
	lastDeleteID   uuid.UUID
	lastReason     *string
	lastCreate     *model.ThreadDialogExtended
}

func (f *fakeThreadDialogStore) Create(ctx context.Context, threadDialog *model.ThreadDialogExtended) (*model.ThreadDialogExtended, error) {
	f.lastCreate = threadDialog
	if threadDialog == nil {
		return nil, nil
	}
	if threadDialog.ID == uuid.Nil {
		threadDialog.BaseModel.ID = uuid.New()
	}
	return threadDialog, nil
}

func (f *fakeThreadDialogStore) Delete(ctx context.Context, memberID uuid.UUID, leaveReason *string) error {
	f.lastDeleteID = memberID
	f.lastReason = leaveReason
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
	return f.initiatorPair, f.targetPair, nil
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

type fakePrivacyChecker struct{}

func (f fakePrivacyChecker) CanInvite(ctx context.Context, initiatorID, targetID uuid.UUID) error {
	return nil
}

func (f fakePrivacyChecker) CanSend(ctx context.Context, senderID, recipientID uuid.UUID) error {
	return nil
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

func TestRemoveMember_ForwardsLeaveReasonToStoreDelete(t *testing.T) {
	targetID := uuid.New()
	threadID := uuid.New()
	reason := "left by own decision"

	threadDialogStore := &fakeThreadDialogStore{
		targetPair: &model.ThreadDialogExtended{
			BaseModel: shared.BaseModel{ID: targetID},
			ThreadID:  threadID,
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{threadDialogStore: threadDialogStore},
	}

	err := svc.RemoveMember(context.Background(), &dto.RemoveMemberRequest{
		TargetMemberID:     targetID,
		InitiatorContactID: uuid.Nil,
		Reason:             &reason,
	})

	require.NoError(t, err)
	require.Equal(t, targetID, threadDialogStore.lastDeleteID)
	require.NotNil(t, threadDialogStore.lastReason)
	require.Equal(t, reason, *threadDialogStore.lastReason)
}

func TestAddMember_SetsInvitedByFromInitiatorDialogID(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	newMemberContactID := uuid.New()
	initiatorMemberID := uuid.New()

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:  shared.BaseModel{ID: initiatorMemberID},
				ContactID:  initiatorContactID,
				ThreadID:   threadID,
				ThreadRole: model.RoleOwner,
				Permissions: model.ThreadPermissions{
					CanAddMembers: true,
				},
				Settings: model.BaseThreadSetting{Title: "Initiator title"},
			},
		},
	}

	svc := &ThreadManagementService{
		uow:            fakeUnitOfWork{threadDialogStore: threadDialogStore},
		privacyChecker: fakePrivacyChecker{},
	}

	_, err := svc.AddMember(context.Background(), &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: newMemberContactID,
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
	})

	require.NoError(t, err)
	require.NotNil(t, threadDialogStore.lastCreate)
	require.NotNil(t, threadDialogStore.lastCreate.InvitedBy)
	require.Equal(t, initiatorMemberID, *threadDialogStore.lastCreate.InvitedBy)
}

var _ store.UnitOfWork = fakeUnitOfWork{}
var _ store.ThreadDialogStore = (*fakeThreadDialogStore)(nil)
var _ store.ThreadStore = nil
var _ store.OutboxStore = fakeOutboxStore{}
var _ = dto.AddMemberRequest{}
