package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/service/guards"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

var (
	_ ThreadManager     = (*thread)(nil)
	_ ThreadProvisioner = (*thread)(nil)
	_ ThreadSearcher    = (*thread)(nil)
)

type (
	ThreadManager interface {
		ThreadProvisioner
		ThreadSearcher
	}

	ThreadProvisioner interface {
		EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error)
	}

	ThreadSearcher interface {
		Search(ctx context.Context, searchRequest *dto.SearchThreadRequest) ([]*model.Thread, error)
	}

	thread struct {
		uow store.UnitOfWork
	}
)

// NewThreadService returns a new thread manager, given a unit of work.
func NewThreadService(uow store.UnitOfWork) *thread {
	return &thread{
		uow: uow,
	}
}

// Search searches for threads based on the given request.
// It returns the threads found by the search, or an error if the operation fails.
// If the operation succeeds, it returns the threads found by the search.
// If the operation fails, it returns nil and the error.
// The request can contain filters on thread id, domain id, kind, member id, owner id, subject, limit, sort, and offset.
// The response contains the threads found by the search.
func (t *thread) Search(ctx context.Context, searchRequest *dto.SearchThreadRequest) ([]*model.Thread, error) {
	if err := guards.SearchThreadValidationGuard(searchRequest); err != nil {
		return nil, err
	}

	query := queryobject.NewThreadQueryObject().
		WithFields(searchRequest.Fields).
		WithIDFilter(searchRequest.Ids...).
		WithDomainIDFilter(searchRequest.DomainIds...).
		WithKindFilter(searchRequest.Kinds...).
		WithMemberIDFilter(searchRequest.MemberIds...).
		WithOwnerFilter(searchRequest.Owners...).
		WithSubjectFilter(searchRequest.Q).
		WithLimit(searchRequest.Limit).
		WithSort(searchRequest.Sort).
		WithOffset(searchRequest.Page)

	threads, err := t.uow.ThreadStore().Search(ctx, query)
	if err != nil {
		return nil, err
	}

	return threads, nil
}

// EnsureDirectThread resolves a direct thread by peers. If the thread is found, it returns the thread id.
// If not found, it creates a new thread and two new thread dialogs for peers within one transaction.
// If any error occurs during the transaction, it rolls back all operations and returns the error.
func (t *thread) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error) {
	var (
		err          error
		directThread *dto.EnsureDirectThreadResponse
	)

	// RESOLVE DIRECT THREAD BY PEERS!
	if directThread, err = t.searchDirectThread(ctx, req); err != nil {
		return nil, err
	}

	// SUCCESSFULLY FOUND!
	if directThread != nil {
		return directThread, nil
	}

	// IF NOT FOUND WE NEED TO CREATE NEW THREAD AND TWO NEW THREAD DIALOG FOR PEERS WITHIN ONE TRANSACTION!
	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		directThread, err = t.createDirectThread(ctx, req, uow)
		// ROLLBACK ALL OPERATION IN CAUSE OF ONE FAILURE!
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return directThread, nil
}

// searchDirectThread resolves a direct thread by peers. If the thread is found, it returns the thread id.
// If not found, it returns nil and the error.
func (t *thread) searchDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error) {
	searchDirectThreadRequest := dto.NewSearchThreadRequest(
		req.DomainID,
		model.ThreadDirect,
		req.PeerFrom,
		req.PeerTo,
	)

	directThreadId, err := t.uow.ThreadDialogStore().Resolve(ctx, searchDirectThreadRequest)
	if err == nil && directThreadId != uuid.Nil {
		return dto.NewEnsureDirectThreadResponse(directThreadId), nil
	}

	return nil, err
}

// createDirectThread creates a new direct thread and two new thread dialogs for peers within one transaction.
// It returns the newly created thread id or an error if the operation fails.
// If the operation succeeds, it returns a new EnsureDirectThreadResponse with the newly created thread id.
// If the operation fails, it returns nil and the error.
func (t *thread) createDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest, uow store.UnitOfWork) (*dto.EnsureDirectThreadResponse, error) {
	var (
		err          error
		now          = time.Now().UTC()
		directThread = &model.Thread{
			BaseModel: shared.BaseModel{
				DomainID:  req.DomainID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Kind: model.ThreadDirect,
			// LEAVE EMPTY SUBJECT FOR DIRECT THREAD!
		}
	)

	if directThread, err = uow.ThreadStore().Create(ctx, directThread); err != nil {
		return nil, err
	}

	dialog := &model.ThreadDialog{
		BaseModel: shared.BaseModel{
			DomainID:  req.DomainID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		MemberID: req.MemberID,
		ThreadID: directThread.ID,
		DirectTo: &req.PeerTo.ID,
	}

	var (
		baseMemberSettings = model.NewBaseThreadSettingBuilder().WithDomainID(req.DomainID).WithTitle(req.PeerFrom.Identity.Name).Build()
		memberSettings     = model.NewDirectThreadSettingBuilder().WithBaseSettings(baseMemberSettings).Build()

		baseDirectToSettings = model.NewBaseThreadSettingBuilder().WithDomainID(req.DomainID).WithTitle(req.PeerTo.Identity.Name).Build()
		directToSettings     = model.NewDirectThreadSettingBuilder().WithBaseSettings(baseDirectToSettings).Build()
	)

	directThreadDialog := model.NewDirectThreadDialogBuilder().
		WithThreadDialog(dialog).
		WithDirectToSettings(directToSettings).
		WithMemberSettings(memberSettings).
		WithDomainID(req.DomainID).
		Build()

	// CREATE TWO RECORDS WITH PAIR member_id <-> direct_to AND REVERSED direct_to <-> member_id and specific user settings
	if _, err = uow.DirectThreadDialogOrchestration().InitializeFullDirectThread(ctx, directThreadDialog); err != nil {
		return nil, err
	}

	return &dto.EnsureDirectThreadResponse{ID: directThread.ID}, nil
}
