package store

import "context"

type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, uow UnitOfWork) error) error
	ThreadDialogStore() ThreadDialogStore
	ThreadStore() ThreadStore
	ThreadPermissionStore() ThreadPermissionStore
	MessageHistory() MessageHistory
	MessageRevisions() MessageRevisionStore
	Messages() MessageStore
	MessageExternal() MessageExternalStore
	MessageStatuses() MessageStatusStore
	Outbox() OutboxStore
	InteractiveCallback() InteractiveCallback
	MessageReactions() MessageReactionStore
	BotControl() BotControlStore
	ThreadTagStore() ThreadTagStore
}
