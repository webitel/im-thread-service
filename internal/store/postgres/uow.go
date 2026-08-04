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

	threadStore                     store.ThreadStore
	threadDialogStore               store.ThreadDialogStore
	threadPermissionsStore          store.ThreadPermissionStore
	messageStore                    store.MessageStore
	messageExternalStore            store.MessageExternalStore
	messageStatusStore              store.MessageStatusStore
	outboxStore                     store.OutboxStore
	messageHistoryStore             store.MessageHistory
	directThreadDialogOrchestration store.DirectThreadDialogOrchestration
	interactiveCallbackStore        store.InteractiveCallback
	messageReactionStore            store.MessageReactionStore
	botControlStore                 store.BotControlStore
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

func (u *unitOfWork) InteractiveCallback() store.InteractiveCallback {
	if u.interactiveCallbackStore == nil {
		u.interactiveCallbackStore = NewInteractiveCallbackStore(u.querier)
	}

	return u.interactiveCallbackStore
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

func (u *unitOfWork) ThreadPermissionStore() store.ThreadPermissionStore {
	if u.threadPermissionsStore == nil {
		u.threadPermissionsStore = NewThreadPermissionStore(u.querier)
	}

	return u.threadPermissionsStore
}

func (u *unitOfWork) Messages() store.MessageStore {
	if u.messageStore == nil {
		u.messageStore = NewMessageStore(u.querier)
	}

	return u.messageStore
}

func (u *unitOfWork) MessageReactions() store.MessageReactionStore {
	if u.messageReactionStore == nil {
		u.messageReactionStore = NewMessageReactionStore(u.querier)
	}

	return u.messageReactionStore
}

func (u *unitOfWork) MessageExternal() store.MessageExternalStore {
	if u.messageExternalStore == nil {
		u.messageExternalStore = NewMessageExternalStore(u.querier)
	}

	return u.messageExternalStore
}

func (u *unitOfWork) MessageStatuses() store.MessageStatusStore {
	if u.messageStatusStore == nil {
		u.messageStatusStore = NewMessageStatusStore(u.querier)
	}

	return u.messageStatusStore
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

func (u *unitOfWork) BotControl() store.BotControlStore {
	if u.botControlStore == nil {
		u.botControlStore = NewBotControlStore(u.querier)
	}

	return u.botControlStore
}

func (u *unitOfWork) DirectThreadDialogOrchestration() store.DirectThreadDialogOrchestration {
	if u.directThreadDialogOrchestration == nil {
		u.directThreadDialogOrchestration = NewDirectThreadDialogOrchestration(u.querier)
	}

	return u.directThreadDialogOrchestration
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
