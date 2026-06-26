package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/infra/pgw"

	"github.com/webitel/im-thread-service/internal/store"
)

type UnitOfWorkFactory struct {
	pool     *pgw.PoolManager
	wmlogger watermill.LoggerAdapter
}

func NewUnitOfWorkFactory(pool *pgw.PoolManager, wmlogger watermill.LoggerAdapter) store.UnitOfWorkFactory {
	return &UnitOfWorkFactory{
		pool:     pool,
		wmlogger: wmlogger,
	}
}

func (f *UnitOfWorkFactory) NewUnitOfWork(ctx context.Context) (store.UnitOfWork, error) {
	pool, err := f.pool.Primary()
	if err != nil {
		return nil, fmt.Errorf("failed to get primary pool: %w", err)
	}
	return &unitOfWork{
		pool:     pool,
		wmlogger: f.wmlogger,
	}, nil
}

type unitOfWork struct {
	pool     *pgw.Pool
	wmlogger watermill.LoggerAdapter
}

type unitOfWorkStore struct {
	querier  pgx.Tx
	wmlogger watermill.LoggerAdapter

	threadStore              store.ThreadStore
	threadDialogStore        store.ThreadDialogStore
	threadPermissionsStore   store.ThreadPermissionStore
	messageStore             store.MessageStore
	outboxStore              store.OutboxStore
	messageHistoryStore      store.MessageHistoryStore
	interactiveCallbackStore store.InteractiveCallback
	botControlStore          store.BotControlStore
}

func (u *unitOfWorkStore) Thread() store.ThreadStore {
	if u.threadStore == nil {
		u.threadStore = NewThreadStore(u.querier)
	}

	return u.threadStore
}

func (u *unitOfWorkStore) ThreadDialog() store.ThreadDialogStore {
	if u.threadDialogStore == nil {
		u.threadDialogStore = NewThreadDialogStore(u.querier)
	}

	return u.threadDialogStore
}

func (u *unitOfWorkStore) MessageHistory() store.MessageHistoryStore {
	if u.messageHistoryStore == nil {
		u.messageHistoryStore = NewMessageHistoryStore(u.querier)
	}

	return u.messageHistoryStore
}

func (u *unitOfWorkStore) InteractiveCallback() store.InteractiveCallback {
	if u.interactiveCallbackStore == nil {
		u.interactiveCallbackStore = NewInteractiveCallbackStore(u.querier)
	}

	return u.interactiveCallbackStore
}

func (u *unitOfWorkStore) ThreadPermission() store.ThreadPermissionStore {
	if u.threadPermissionsStore == nil {
		u.threadPermissionsStore = NewThreadPermissionStore(u.querier)
	}

	return u.threadPermissionsStore
}

func (u *unitOfWorkStore) Messages() store.MessageStore {
	if u.messageStore == nil {
		u.messageStore = NewMessageStore(u.querier)
	}

	return u.messageStore
}

func (u *unitOfWorkStore) Outbox() store.OutboxStore {
	if u.outboxStore == nil {
		u.outboxStore = NewOutboxStore(u.querier, u.wmlogger)
	}

	return u.outboxStore
}

func (u *unitOfWorkStore) BotControl() store.BotControlStore {
	if u.botControlStore == nil {
		u.botControlStore = NewBotControlStore(u.querier)
	}

	return u.botControlStore
}

// WithinTransaction executes a function within a transaction.
// If the function panics, WithinTransaction will rollback all changes.
// If the function returns an error, WithinTransaction will rollback all changes
// and return the error.
// If the function returns nil, WithinTransaction will commit all changes.
// Nested transactions are not supported, WithinTransaction will return an error
// if the given uow is already part of an open transaction.
func (u *unitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, uow store.UnitOfWorkStore) error) error {

	// [BEGIN]
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	uowStore := &unitOfWorkStore{
		querier:  tx,
		wmlogger: u.wmlogger,
	}

	// ROLLBACK CHANGES IN CASE OF PANIC
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx) // [ROLLBACK]

			panic(p)
		}
	}()

	// EXECUTE CALLBACK FUNCTION WITHIN ONE TRANSACTION!
	if err := fn(ctx, uowStore); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil { // [ROLLBACK]
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rbErr)
		}

		return err
	}

	// [COMMIT]
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
