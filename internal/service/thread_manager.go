package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

var _ ThreadManager = (*thread)(nil)

type (
	ThreadManager interface {
		EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error)
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
		MemberID: req.MemberID, // member id == peer.from.id or not?
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

	if _, err = uow.DirectThreadDialogOrchestration().InitializeFullDirectThread(ctx, directThreadDialog); err != nil {
		return nil, err
	}

	// // CREATE TWO RECORDS WITH PAIR member_id <-> direct_to AND REVERSED direct_to <-> member_id
	// if _, err = uow.ThreadDialogStore().CreateDirectPair(ctx, dialog); err != nil {
	// 	return nil, err
	// }

	return &dto.EnsureDirectThreadResponse{ID: directThread.ID}, nil
}
