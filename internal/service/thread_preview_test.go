package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestGivePreview_CreatesPreviewWithoutMemberSideEffects(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	contactID := uuid.New()
	initiatorID := uuid.New()
	domainID := 1

	fake := &fakeThreadPreviewStore{
		canRead: true,
	}
	fakeThreads := &fakeThreadStore{}
	fakeDialogs := &fakeThreadDialogStore{
		fullViewResult: nil, // no existing member
	}
	fakeOutbox := &fakeOutboxStore{}

	uow := fakeUnitOfWork{
		threadPreviewStore: fake,
		threadStore:        fakeThreads,
		threadDialogStore:  fakeDialogs,
		outboxStore:        fakeOutbox,
	}

	svc := &ThreadPreviewService{
		uow:    uow,
		logger: slog.Default(),
		members: &ThreadManagementService{
			uow:            uow,
			logger:         slog.Default(),
			privacyChecker: fakePrivacyChecker{},
		},
	}

	req := &dto.GivePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           domainID,
		Duration:           time.Hour,
	}

	preview, err := svc.GivePreview(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, fake.created, preview)
	require.Equal(t, contactID, preview.ContactID)
	require.Equal(t, initiatorID, preview.InitiatorID)
	require.Equal(t, threadID, preview.ThreadID)
	require.Equal(t, time.Hour, fake.createdTTL)
}

func TestGivePreview_ExtendsActivePreview(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	contactID := uuid.New()
	initiatorID := uuid.New()
	domainID := 1

	existingPreview := &model.ThreadPreview{
		ID:        uuid.New(),
		ThreadID:  threadID,
		ContactID: contactID,
		DomainID:  domainID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	fake := &fakeThreadPreviewStore{
		active:  existingPreview,
		canRead: true,
	}
	fakeThreads := &fakeThreadStore{}
	fakeDialogs := &fakeThreadDialogStore{
		fullViewResult: nil,
	}

	uow := fakeUnitOfWork{
		threadPreviewStore: fake,
		threadStore:        fakeThreads,
		threadDialogStore:  fakeDialogs,
	}

	svc := &ThreadPreviewService{
		uow:    uow,
		logger: slog.Default(),
		members: &ThreadManagementService{
			uow:    uow,
			logger: slog.Default(),
		},
	}

	req := &dto.GivePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           domainID,
		Duration:           30 * time.Minute,
	}

	preview, err := svc.GivePreview(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, existingPreview.ID, preview.ID)
	require.Equal(t, fake.extendedID, existingPreview.ID)
	require.Equal(t, fake.extendedTTL, 30*time.Minute)
	require.Nil(t, fake.created) // no new preview created
}

func TestGivePreview_RejectsExistingMember(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	contactID := uuid.New()
	initiatorID := uuid.New()
	domainID := 1

	fakeDialogs := &fakeThreadDialogStore{
		quickViewResult: []*model.ThreadDialog{
			{
				BaseModel: shared.BaseModel{DomainID: domainID},
				ThreadID:  threadID,
				ContactID: contactID,
			},
		},
	}
	fakeThreads := &fakeThreadStore{}

	uow := fakeUnitOfWork{
		threadStore:       fakeThreads,
		threadDialogStore: fakeDialogs,
		threadPreviewStore: &fakeThreadPreviewStore{
			canRead: true,
		},
	}

	svc := &ThreadPreviewService{
		uow:    uow,
		logger: slog.Default(),
		members: &ThreadManagementService{
			uow:    uow,
			logger: slog.Default(),
		},
	}

	req := &dto.GivePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           domainID,
		Duration:           time.Hour,
	}

	_, err := svc.GivePreview(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.AlreadyExists, errors.Code(err))
}

func TestGivePreview_RejectsDurationOverMax(t *testing.T) {
	ctx := context.Background()

	svc := &ThreadPreviewService{
		logger: slog.Default(),
	}

	req := &dto.GivePreviewRequest{
		ThreadID:           uuid.New(),
		ContactID:          uuid.New(),
		InitiatorContactID: uuid.New(),
		DomainID:           1,
		Duration:           2 * time.Hour, // > 1 hour max
	}

	_, err := svc.GivePreview(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestGivePreview_RejectsNonPositiveDuration(t *testing.T) {
	ctx := context.Background()

	svc := &ThreadPreviewService{
		logger: slog.Default(),
	}

	req := &dto.GivePreviewRequest{
		ThreadID:           uuid.New(),
		ContactID:          uuid.New(),
		InitiatorContactID: uuid.New(),
		DomainID:           1,
		Duration:           0, // non-positive
	}

	_, err := svc.GivePreview(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestRemoveFromPreview_RevokesWithRemoveReason(t *testing.T) {
	ctx := context.Background()
	previewID := uuid.New()
	threadID := uuid.New()
	contactID := uuid.New()
	domainID := 1

	preview := &model.ThreadPreview{
		ID:        previewID,
		ThreadID:  threadID,
		ContactID: contactID,
		DomainID:  domainID,
	}

	fake := &fakeThreadPreviewStore{
		active: preview,
	}

	uow := fakeUnitOfWork{
		threadPreviewStore: fake,
	}

	svc := &ThreadPreviewService{
		uow:    uow,
		logger: slog.Default(),
	}

	req := &dto.RemoveFromPreviewRequest{
		ThreadID:  threadID,
		ContactID: contactID,
		DomainID:  domainID,
	}

	err := svc.RemoveFromPreview(ctx, req)
	require.NoError(t, err)
	require.Equal(t, previewID, fake.revokedID)
	require.Equal(t, model.PreviewRevokeReasonRemove, fake.revokedReason)
}

func TestRemoveFromPreview_NoActivePreview(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	contactID := uuid.New()
	domainID := 1

	fake := &fakeThreadPreviewStore{
		active: nil, // no active preview
	}

	uow := fakeUnitOfWork{
		threadPreviewStore: fake,
	}

	svc := &ThreadPreviewService{
		uow:    uow,
		logger: slog.Default(),
	}

	req := &dto.RemoveFromPreviewRequest{
		ThreadID:  threadID,
		ContactID: contactID,
		DomainID:  domainID,
	}

	err := svc.RemoveFromPreview(ctx, req)
	require.NoError(t, err) // idempotent: no error
}

func TestUpgradePreview_RevokesAndAddsMemberWithEvents(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	contactID := uuid.New()
	initiatorID := uuid.New()
	domainID := 1

	previewID := uuid.New()
	preview := &model.ThreadPreview{
		ID:        previewID,
		ThreadID:  threadID,
		ContactID: contactID,
		DomainID:  domainID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	fakePreview := &fakeThreadPreviewStore{
		active: preview,
	}
	fakeDialogs := &fakeThreadDialogStore{
		fullViewResult: nil, // not already a member
		quickViewResult: []*model.ThreadDialog{
			{
				BaseModel: shared.BaseModel{ID: uuid.New()},
				ThreadID:  threadID,
				ContactID: uuid.New(), // other member
			},
		},
	}
	fakeThreads := &fakeThreadStore{}
	fakeOutbox := &fakeOutboxStore{}
	fakeMessages := &fakeMessageStore{}

	uow := fakeUnitOfWork{
		threadPreviewStore: fakePreview,
		threadDialogStore:  fakeDialogs,
		threadStore:        fakeThreads,
		outboxStore:        fakeOutbox,
		messageStore:       fakeMessages,
	}

	members := &ThreadManagementService{
		uow:            uow,
		logger:         slog.Default(),
		privacyChecker: fakePrivacyChecker{},
	}

	svc := &ThreadPreviewService{
		uow:     uow,
		logger:  slog.Default(),
		members: members,
	}

	req := &dto.UpgradePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           domainID,
		Role:               model.RoleOwner,
	}

	id, err := svc.UpgradePreviewToFullMember(ctx, req)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)
	require.Equal(t, previewID, fakePreview.revokedID)
	require.Equal(t, model.PreviewRevokeReasonUpgrade, fakePreview.revokedReason)
	require.NotNil(t, fakeDialogs.lastCreate)
	require.Equal(t, contactID, fakeDialogs.lastCreate.ContactID)
	require.Equal(t, model.RoleOwner, fakeDialogs.lastCreate.ThreadRole)
}

func TestUpgradePreview_InactivePreviewChangesNothing(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	contactID := uuid.New()
	initiatorID := uuid.New()
	domainID := 1

	fakePreview := &fakeThreadPreviewStore{
		active: nil, // no active preview
	}
	fakeDialogs := &fakeThreadDialogStore{
		fullViewResult: nil,
	}
	fakeThreads := &fakeThreadStore{}

	uow := fakeUnitOfWork{
		threadPreviewStore: fakePreview,
		threadDialogStore:  fakeDialogs,
		threadStore:        fakeThreads,
	}

	members := &ThreadManagementService{
		uow:    uow,
		logger: slog.Default(),
	}

	svc := &ThreadPreviewService{
		uow:     uow,
		logger:  slog.Default(),
		members: members,
	}

	req := &dto.UpgradePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           domainID,
		Role:               model.RoleOwner,
	}

	_, err := svc.UpgradePreviewToFullMember(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.FailedPrecondition, errors.Code(err))
	require.Nil(t, fakeDialogs.lastCreate) // no member created
}

func TestUpgradePreview_RejectsUnspecifiedRole(t *testing.T) {
	ctx := context.Background()

	svc := &ThreadPreviewService{
		logger: slog.Default(),
	}

	req := &dto.UpgradePreviewRequest{
		ThreadID:           uuid.New(),
		ContactID:          uuid.New(),
		InitiatorContactID: uuid.New(),
		DomainID:           1,
		Role:               model.UnspecifiedRole,
	}

	_, err := svc.UpgradePreviewToFullMember(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestGet_CallerWithoutAccessIsForbidden(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	callerID := uuid.New()
	domainID := 1

	fakePreview := &fakeThreadPreviewStore{
		canRead: false, // no access
	}

	uow := fakeUnitOfWork{
		threadPreviewStore: fakePreview,
	}

	svc := &ThreadManagementService{
		uow:    uow,
		logger: slog.Default(),
	}

	req := &dto.ThreadGetRequest{
		ID:       threadID,
		DomainID: domainID,
		CallerID: callerID, // access check required
	}

	_, err := svc.Get(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, errors.Code(err))
}

func TestGet_NoCallerSkipsAccessCheck(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	domainID := 1

	expectedThread := &model.Thread{
		ID:       threadID,
		DomainID: domainID,
	}

	fakeStore := &fakeThreadStore{
		getResult: expectedThread,
	}

	uow := fakeUnitOfWork{
		threadStore: fakeStore,
	}

	svc := &ThreadManagementService{
		uow:    uow,
		logger: slog.Default(),
	}

	req := &dto.ThreadGetRequest{
		ID:       threadID,
		DomainID: domainID,
		CallerID: uuid.Nil, // no access check
	}

	thread, err := svc.Get(ctx, req)
	require.NoError(t, err)
	require.Equal(t, expectedThread, thread)
}

func TestSearch_RejectsNilSelfID(t *testing.T) {
	ctx := context.Background()

	svc := &ThreadManagementService{
		logger: slog.Default(),
	}

	req := &dto.ThreadSearchRequest{
		SelfID: uuid.Nil, // invalid
	}

	_, err := svc.Search(ctx, req)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestLocate_WithoutCallerSkipsAccessCheck(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()

	fakeVarsStore := &noopThreadVariablesStore{}
	fakePreview := &fakeThreadPreviewStore{
		canRead: false, // would normally deny access
	}

	svc := &threadVariables{
		store:     fakeVarsStore,
		previews:  fakePreview,
		logger:    slog.Default(),
		publisher: nil,
	}

	query := model.LocateThreadVariablesQuery{
		ThreadID: threadID,
		CallerID: uuid.Nil, // no access check
	}

	vars, err := svc.Locate(ctx, query)
	// Should succeed (no access check) despite canRead being false
	require.NoError(t, err)
	require.Nil(t, vars)
}

func TestLocate_WithCallerDeniesAccessIfNotReadable(t *testing.T) {
	ctx := context.Background()
	threadID := uuid.New()
	callerID := uuid.New()

	fakeVarsStore := &noopThreadVariablesStore{}
	fakePreview := &fakeThreadPreviewStore{
		canRead: false, // deny access
	}

	svc := &threadVariables{
		store:     fakeVarsStore,
		previews:  fakePreview,
		logger:    slog.Default(),
		publisher: nil,
	}

	query := model.LocateThreadVariablesQuery{
		ThreadID: threadID,
		CallerID: callerID, // access check required
	}

	_, err := svc.Locate(ctx, query)
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, errors.Code(err))
}
