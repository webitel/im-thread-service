package store

import "context"

type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, uow UnitOfWork) error) error
	ThreadDialogStore() ThreadDialogStore
	ThreadStore() ThreadStore
	MessageHistory() MessageHistory
	Messages() MessageStore
	Outbox() OutboxStore
	DirectThreadDialogOrchestration() DirectThreadDialogOrchestration
	InteractiveCallback() InteractiveCallback
}
