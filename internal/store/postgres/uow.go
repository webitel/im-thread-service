package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/webitel/im-thread-service/internal/store"
)

type unitOfWork struct {
	pool     *pgxpool.Pool
	querier  Querier
	wmlogger watermill.LoggerAdapter

	threadStore       store.ThreadStore
	threadDialogStore store.ThreadDialogStore
	messageStore      store.MessageStore
	outboxStore       store.OutboxStore
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

// ThreadStore returns the thread store for the given unit of work.
// It caches the underlying store, so subsequent calls with the same unit of work will return the same store instance.
// If the store is not cached, it creates a new store by calling NewThreadStore with the given querier.
// The store is used to resolve a direct thread by peers and to create a new direct thread with peers within one transaction.
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
		u.messageStore = NewMessageStore(u.querier)
	}
	return u.messageStore
}

func (u *unitOfWork) Outbox() store.OutboxStore {
	if u.outboxStore == nil {
		u.outboxStore = NewOutboxStore(u.querier, u.wmlogger)
	}
	return u.outboxStore
}

// WithinTransaction executes a function within a transaction.
// If the function panics, WithinTransaction will rollback all changes.
// If the function returns an error, WithinTransaction will rollback all changes
// and return the error.
// If the function returns nil, WithinTransaction will commit all changes.
// Nested transactions are not supported, WithinTransaction will return an error
// if the given uow is already part of an open transaction.
func (u *unitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, uow store.UnitOfWork) error) error {
	// NESTED TRANSACTION ARE NOT SUPPORTED, RETURN!
	if _, ok := u.querier.(pgx.Tx); ok {
		return fn(ctx, u)
	}

	// [BEGIN]
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	txUow := &unitOfWork{
		pool:    u.pool,
		querier: tx, // SET OPENED TRANSACTION AS QUERY CONTEXT
	}

	// ROLLBACK CHANGES IN CASE OF PANIC
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx) // [ROLLBACK]
			panic(p)
		}
	}()

	// EXECUTE CALLBACK FUNCTION WITHIN ONE TRANSACTION!
	if err := fn(ctx, txUow); err != nil {
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
