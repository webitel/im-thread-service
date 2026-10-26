package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// queueTestSetup returns an initiator (owner with add-members rights) already in the thread.
func queueTestSetup(threadID, initiatorContactID, initiatorMemberID uuid.UUID) *fakeThreadDialogStore {
	return &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:   initiatorContactID,
				ThreadID:    threadID,
				ThreadRole:  model.RoleOwner,
				Permissions: model.ThreadPermissions{CanAddMembers: true},
				Settings:    model.BaseThreadSetting{Title: "Initiator title"},
			},
		},
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: initiatorMemberID}, ContactID: initiatorContactID, ThreadID: threadID, ThreadRole: model.RoleOwner},
		},
	}
}

func TestAddMember_WithQueueWritesQueueIDAndRef(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadID, initiatorContactID, initiatorMemberID := uuid.New(), uuid.New(), uuid.New()
	dialogs := queueTestSetup(threadID, initiatorContactID, initiatorMemberID)
	refStore := &fakeReferenceStore{}
	refs := newTestReferenceCache()

	svc := &ThreadManagementService{
		tagHooks:       &fakeTagHooks{},
		uow:            fakeUnitOfWork{threadDialogStore: dialogs, messageStore: &fakeMessageStore{}, outboxStore: &fakeOutboxStore{}, referenceStore: refStore},
		privacyChecker: fakePrivacyChecker{},
		refs:           refs,
	}

	_, err := svc.AddMember(ctx, &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: uuid.New(),
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
		Queue:              &model.QueueInfo{ID: 7, Name: "Sales"},
	})
	require.NoError(t, err)
	require.NotNil(t, dialogs.lastCreate.QueueID)
	require.Equal(t, int64(7), *dialogs.lastCreate.QueueID)
	require.Equal(t, []*model.QueueRef{{DomainID: 1, QueueID: 7, Name: "Sales"}}, refStore.queues)
	require.False(t, refs.QueueChanged(ctx, refStore.queues[0]), "committed queue must be remembered")
}

func TestAddMember_WithoutQueueKeepsQueueIDNullAndWritesNoRef(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadID, initiatorContactID, initiatorMemberID := uuid.New(), uuid.New(), uuid.New()
	dialogs := queueTestSetup(threadID, initiatorContactID, initiatorMemberID)
	refStore := &fakeReferenceStore{}

	svc := &ThreadManagementService{
		tagHooks:       &fakeTagHooks{},
		uow:            fakeUnitOfWork{threadDialogStore: dialogs, messageStore: &fakeMessageStore{}, outboxStore: &fakeOutboxStore{}, referenceStore: refStore},
		privacyChecker: fakePrivacyChecker{},
		refs:           newTestReferenceCache(),
	}

	_, err := svc.AddMember(ctx, &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: uuid.New(),
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
	})
	require.NoError(t, err)
	require.Nil(t, dialogs.lastCreate.QueueID)
	require.Empty(t, refStore.queues)
}

func TestAddMember_QueueWithoutNameSetsQueueIDOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadID, initiatorContactID, initiatorMemberID := uuid.New(), uuid.New(), uuid.New()
	dialogs := queueTestSetup(threadID, initiatorContactID, initiatorMemberID)
	refStore := &fakeReferenceStore{}

	svc := &ThreadManagementService{
		tagHooks:       &fakeTagHooks{},
		uow:            fakeUnitOfWork{threadDialogStore: dialogs, messageStore: &fakeMessageStore{}, outboxStore: &fakeOutboxStore{}, referenceStore: refStore},
		privacyChecker: fakePrivacyChecker{},
		refs:           newTestReferenceCache(),
	}

	_, err := svc.AddMember(ctx, &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: uuid.New(),
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
		Queue:              &model.QueueInfo{ID: 9, Name: ""},
	})
	require.NoError(t, err)
	require.NotNil(t, dialogs.lastCreate.QueueID)
	require.Equal(t, int64(9), *dialogs.lastCreate.QueueID)
	require.Empty(t, refStore.queues)
}

func TestTransfer_QueueOnlyFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queue     *model.QueueInfo
		wantID    *int64
		wantQueue int
	}{
		{name: "with queue", queue: &model.QueueInfo{ID: 7, Name: "Sales"}, wantID: ptrInt64(7), wantQueue: 1},
		{name: "without queue", wantID: nil, wantQueue: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			threadID, initiatorContactID, initiatorMemberID := uuid.New(), uuid.New(), uuid.New()
			dialogs := queueTestSetup(threadID, initiatorContactID, initiatorMemberID)
			refStore := &fakeReferenceStore{}

			svc := &ThreadManagementService{
				tagHooks:       &fakeTagHooks{},
				uow:            fakeUnitOfWork{threadDialogStore: dialogs, messageStore: &fakeMessageStore{}, outboxStore: &fakeOutboxStore{}, referenceStore: refStore},
				privacyChecker: fakePrivacyChecker{},
				refs:           newTestReferenceCache(),
			}

			_, err := svc.Transfer(ctx, &dto.TransferThreadRequest{
				ThreadID:           threadID,
				NewMemberContactID: uuid.New(),
				InitiatorContactID: initiatorContactID,
				NewMemberRole:      model.RoleMember,
				Queue:              tt.queue,
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantID, dialogs.lastCreate.QueueID)
			require.Len(t, refStore.queues, tt.wantQueue)
		})
	}
}

func TestUpgradePreview_PassesQueueThroughToMember(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	threadID, contactID, initiatorID := uuid.New(), uuid.New(), uuid.New()
	refStore := &fakeReferenceStore{}
	dialogs := &fakeThreadDialogStore{
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: uuid.New(), ThreadID: threadID, ThreadRole: model.RoleOwner},
		},
	}

	uow := fakeUnitOfWork{
		threadPreviewStore: &fakeThreadPreviewStore{active: &model.ThreadPreview{
			ID: uuid.New(), ThreadID: threadID, ContactID: contactID, DomainID: 1,
		}},
		threadDialogStore: dialogs,
		threadStore:       &fakeThreadStore{},
		outboxStore:       &fakeOutboxStore{},
		messageStore:      &fakeMessageStore{},
		referenceStore:    refStore,
	}

	members := &ThreadManagementService{
		tagHooks:       &fakeTagHooks{},
		uow:            uow,
		logger:         slog.Default(),
		privacyChecker: fakePrivacyChecker{},
		refs:           newTestReferenceCache(),
	}
	svc := &ThreadPreviewService{uow: uow, logger: slog.Default(), members: members}

	_, err := svc.UpgradePreviewToFullMember(ctx, &dto.UpgradePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           1,
		Role:               model.RoleMember,
		Queue:              &model.QueueInfo{ID: 3, Name: "Support"},
	})
	require.NoError(t, err)
	require.NotNil(t, dialogs.lastCreate.QueueID)
	require.Equal(t, int64(3), *dialogs.lastCreate.QueueID)
	require.Equal(t, []*model.QueueRef{{DomainID: 1, QueueID: 3, Name: "Support"}}, refStore.queues)
}

func ptrInt64(v int64) *int64 {
	return &v
}
