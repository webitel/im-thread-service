package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

//go:generate mockgen -destination=./mocks/mocks.go . 

type MessageStoreFactory interface {
	NewMessageStore(ctx context.Context) (MessageStore, error)
}

type MessageStore interface {
	SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveImages(ctx context.Context, messageID uuid.UUID, images []*model.MessageImage) ([]*model.MessageImage, error)
	SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error)
	ReadMessage(ctx context.Context, read struct {
		DomainID  int32
		ThreadID  uuid.UUID
		MessageID uuid.UUID
		UserID    uuid.UUID
	}) error
	SaveMessageContact(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveMessageLocation(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveInteractiveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
}

type OutboxStoreFactory interface {
	NewOutboxStore(ctx context.Context) (OutboxStore, error)
}

type OutboxStore interface {
	Publish(ctx context.Context, topic string, event event.Outboxer) error
	Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error)
}

type ThreadDialogStoreFactory interface {
	NewThreadDialogStore(ctx context.Context) (ThreadDialogStore, error)
}

type ThreadDialogStore interface {
	Create(ctx context.Context, threadDialog *model.ThreadDialogExtended) (*model.ThreadDialogExtended, error)
	Delete(ctx context.Context, memberID uuid.UUID, leaveReason *string) error
	GetQuickView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error)
	GetFullView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialogExtended, error)
	FindActorsPair(ctx context.Context, initiatorsContact, targetMember uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error)
}

type ThreadStoreFactory interface {
	NewThreadStore(ctx context.Context) (ThreadStore, error)
}

type ThreadStore interface {
	Create(ctx context.Context, req *model.Thread) (*model.Thread, error)
	Get(ctx context.Context, query queryobject.QueryObject) (*model.Thread, error)
	Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error)
	ResolveThread(ctx context.Context, q model.ResolveThreadQuery) (*model.Thread, error)
	SearchLeft(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error)
}

type ThreadPermissionStoreFactory interface {
	NewThreadPermissionStore(ctx context.Context) (ThreadPermissionStore, error)
}

type ThreadPermissionStore interface {
	Create(ctx context.Context, in *model.ThreadPermission) (*model.ThreadPermission, error)
	Get(ctx context.Context, in *model.ThreadPermissionStoreFilters) ([]*model.ThreadPermission, error)
	Update(ctx context.Context, in *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error)
}

type MessageHistoryStoreFactory interface {
	NewMessageHistoryStore(ctx context.Context) (MessageHistoryStore, error)
}

type MessageHistoryStore interface {
	Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Message, error)
}

type InteractiveCallbackStoreFactory interface {
	NewInteractiveCallbackStore(ctx context.Context) (InteractiveCallback, error)
}

type InteractiveCallback interface {
	Save(ctx context.Context, callback *model.InteractiveCallback) (*model.InteractiveCallback, error)
}

type ThreadVariablesStoreFactory interface {
	NewThreadVariablesStore(ctx context.Context) (ThreadVariablesStore, error)
}

type ThreadVariablesStore interface {
	Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error)
	Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error)
	Locate(ctx context.Context, threadID uuid.UUID) (*model.ThreadVariables, error)
	Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error)
}

type BotControlStoreFactory interface {
	NewBotControlStore(ctx context.Context) (BotControlStore, error)
}

type BotControlStore interface {
	// Push adds a new entry onto the stack and updates thread.bot_controller_id.
	// Returns the previous top entry.
	Push(ctx context.Context, transition model.BotControlTransition) (*model.BotControlPushResult, error)

	// Pop removes the stack entry for the given memberID (thread_dialog.id).
	// If the member was the current top (max position), updates bot_controller_id to the new top.
	// If the member is marked auto_leave, it is soft-deleted.
	// Returns the new top entry after removal (nil if stack is now empty and no owner bot).
	Pop(ctx context.Context, threadID, memberID uuid.UUID, reason model.BotControlReason, triggeredBy *uuid.UUID) (*model.BotControlStackEntry, error)

	// GetStack returns all entries for a thread ordered by position asc.
	GetStack(ctx context.Context, threadID uuid.UUID) ([]*model.BotControlStackEntry, error)
}
