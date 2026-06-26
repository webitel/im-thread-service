package store

import "context"

type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, uow UnitOfWorkStore) error) error
}

type UnitOfWorkStore interface {
	ThreadDialog() ThreadDialogStore
	Thread() ThreadStore
	ThreadPermission() ThreadPermissionStore
	MessageHistory() MessageHistoryStore
	Messages() MessageStore
	Outbox() OutboxStore
	InteractiveCallback() InteractiveCallback
	BotControl() BotControlStore
}

type UnitOfWorkFactory interface {
	NewUnitOfWork(ctx context.Context) (UnitOfWork, error)
}
