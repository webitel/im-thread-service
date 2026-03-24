package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/im-thread-service/internal/store/querier/sqlbuilder"
)

type unitOfWork struct {
	pool     *pgxpool.Pool
	querier  Querier
	wmlogger watermill.LoggerAdapter

	threadStore                     store.ThreadStore
	threadDialogStore               store.ThreadDialogStore
	messageStore                    store.MessageStore
	outboxStore                     store.OutboxStore
	messageHistoryStore             store.MessageHistory
	directThreadDialogOrchestration store.DirectThreadDialogOrchestration
	directSettings                  store.DirectSettings
	buttonsCallback store.ButtonsCallback
}

// NewPgxUnitOfWork returns a new unit of work, given a pgx pool.
// The unit of work contains the pool and a querier which is used to execute queries.
// The thread store and thread dialog store are lazily initialized on first call to ThreadStore or ThreadDialogStore.
func NewPgxUnitOfWork(pool *pgxpool.Pool, wmlogger watermill.LoggerAdapter) *unitOfWork {
	return &unitOfWork{
		pool:     pool,
		querier:  pool,
		wmlogger: wmlogger,
	}
}

func (u *unitOfWork) ButtonsCallback() store.ButtonsCallback {
	if u.buttonsCallback == nil {
		u.buttonsCallback = NewButtonsCallback(u.querier)
	}

	return u.buttonsCallback
} 

func (u *unitOfWork) ThreadStore() store.ThreadStore {
	if u.threadStore == nil {
		u.threadStore = NewThreadStore(u.querier)
	}

	return u.threadStore
}

// ThreadDialogStore returns the thread dialog store for the given unit of work.
// It caches the underlying store, so subsequent calls with the same unit of work will return the same store instance.
// If the store is not cached, it creates a new store by calling NewThreadDialogStore with the given querier.
// The store is used to resolve a direct thread by peers and to create a new direct thread with peers within one transaction.
func (u *unitOfWork) ThreadDialogStore() store.ThreadDialogStore {
	if u.threadDialogStore == nil {
		u.threadDialogStore = NewThreadDialogStore(u.querier)
	}

	return u.threadDialogStore
}

func (u *unitOfWork) Messages() store.MessageStore {
	if u.messageStore == nil {
		u.messageStore = NewMessageStore(u.querier, sqlbuilder.NewMessageQuerier())
	}
	return u.messageStore
}

func (u *unitOfWork) Outbox() store.OutboxStore {
	if u.outboxStore == nil {
		u.outboxStore = NewOutboxStore(u.querier, u.wmlogger)
	}
	return u.outboxStore
}

func (u *unitOfWork) MessageHistory() store.MessageHistory {
	if u.messageHistoryStore == nil {
		u.messageHistoryStore = NewMessageHistoryStore(u.querier)
	}

	return u.messageHistoryStore
}

func (u *unitOfWork) DirectThreadDialogOrchestration() store.DirectThreadDialogOrchestration {
	if u.directThreadDialogOrchestration == nil {
		u.directThreadDialogOrchestration = NewDirectThreadDialogOrchestration(u.querier)
	}

	return u.directThreadDialogOrchestration
}

func (u *unitOfWork) DirectSettingsStore() store.DirectSettings {
	if u.directSettings == nil {
		u.directSettings = NewDirectSettingsStore(u.querier)
	}

	return u.directSettings
}

func (u *unitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, uow store.UnitOfWork) error) error {
	if _, ok := u.querier.(pgx.Tx); ok {
		return fn(ctx, u)
	}

	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	txUow := &unitOfWork{
		pool:    u.pool,
		querier: tx,
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx) // [ROLLBACK]
			panic(p)
		}
	}()

	if err := fn(ctx, txUow); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
