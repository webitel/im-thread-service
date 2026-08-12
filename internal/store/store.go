package store

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

// ErrReplyTargetNotFound is returned by MessageStore.GetReplyPreview when no
// message matches the requested id and domain.
var ErrReplyTargetNotFound = errors.New("reply target message not found")

// ErrReactionNotAllowed is returned by MessageReactionStore.SetReaction when the
// reactor may not react: the message does not exist in the domain, is deleted,
// or the reactor is not an active member holding can_react_messages.
var ErrReactionNotAllowed = errors.New("reaction not allowed")

// ErrMessageNotVisible is returned by MessageRevisionStore.Search when the
// message does not exist in the domain or the caller is not an active member
// of its thread.
var ErrMessageNotVisible = errors.New("message not visible to caller")

type Store interface {
	Messages() MessageStore
	Outbox() OutboxStore
	ThreadDialog() ThreadDialogStore
	Thread() ThreadStore
	BotControl() BotControlStore
}

type MessageStore interface {
	SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
	GetReplyPreview(ctx context.Context, id uuid.UUID, domainID int32) (*model.ReplyToPreview, error)
	SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error)

	// CopyAttachments duplicates every document and image row of sourceID onto
	// targetID. Nothing is re-uploaded: the copies reference the same stored
	// files by file_id.
	CopyAttachments(ctx context.Context, sourceID, targetID uuid.UUID) error

	// LoadForwardSources returns the messages callerID may forward, with their
	// content already assembled. Ids the caller cannot read, ids that do not
	// exist, deleted messages and system messages are silently omitted, so the
	// result may be shorter than ids or empty, which is not an error at this
	// layer. Rows come back oldest-first so copies keep the original order.
	LoadForwardSources(ctx context.Context, ids []uuid.UUID, callerID uuid.UUID, domainID int32) ([]*model.Message, error)

	SaveMessageContact(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveMessageLocation(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveInteractiveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
	SaveSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error)

	EditMessage(ctx context.Context, msg *model.Message) (*model.Message, error)

	// DeleteMessages soft-deletes the given messages on behalf of deleterID.
	// Only messages authored by the deleter, in threads the deleter is still a
	// member of and still holds can_delete_messages in, are affected, so one
	// batch may be partly refused. The result also holds messages an earlier
	// call already deleted, flagged with JustDeleted=false; it may be shorter
	// than ids or empty, which is not an error at this layer.
	DeleteMessages(ctx context.Context, ids []uuid.UUID, deleterID uuid.UUID) ([]*model.Message, error)
}

// MessageStatusStore tracks per-recipient delivery states of messages
// (im_message.message_statuses). All transitions are monotonic upserts:
// duplicate and out-of-order receipts change nothing and return no changes.
type MessageStatusStore interface {
	// InsertSent creates SENT rows for the message recipients within the
	// message-save transaction. Existing rows are left intact.
	InsertSent(ctx context.Context, msg *model.Message, recipientIDs []uuid.UUID) error

	// MarkDelivered applies delivery receipts and returns the rows that
	// actually changed.
	MarkDelivered(ctx context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error)

	// MarkRead applies read receipts with read-up-to semantics and returns
	// the rows that actually changed.
	MarkRead(ctx context.Context, receipts []*model.ReadReceipt) ([]*model.StatusChange, error)

	// MarkFailed applies failure receipts (sent -> failed only) and returns
	// the rows that actually changed.
	MarkFailed(ctx context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error)

	// ReadUnread returns the denormalized unread_count per thread for the
	// member, read straight from thread_dialog (no message scan). Threads with
	// no active row for the member are omitted.
	ReadUnread(ctx context.Context, domainID int32, memberID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID]int64, error)

	// UnreadSummary returns the member's denormalized unread totals across the
	// threads they are still an active participant of: the number of chats with
	// unread messages and the total number of unread messages.
	UnreadSummary(ctx context.Context, domainID int32, memberID uuid.UUID) (model.UnreadSummary, error)

	// ReconcileUnread recomputes unread_count for every active dialog from the
	// read horizon (optionally scoped to a domain). Drift safety net for a
	// periodic job; returns the number of rows updated.
	ReconcileUnread(ctx context.Context, domainID int32) (int64, error)
}

type OutboxStore interface {
	Publish(ctx context.Context, topic string, event event.Outboxer) error
	Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error)
}

type MessageExternalStore interface {
	Save(ctx context.Context, rec *model.MessageExternalID) error
	LookupMessageID(ctx context.Context, gateID, externalID string) (uuid.UUID, error)
	LookupExternalID(ctx context.Context, messageID uuid.UUID, gateID string) (string, error)
	UpdateDelivery(ctx context.Context, in *model.MessageDelivery) (*model.MessageExternalID, error)
}

type ThreadDialogStore interface {
	Create(ctx context.Context, threadDialog *model.ThreadDialogExtended) (*model.ThreadDialogExtended, error)
	Delete(ctx context.Context, memberID uuid.UUID, leaveReason *string) error
	GetQuickView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error)
	GetFullView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialogExtended, error)
	FindActorsPair(ctx context.Context, initiatorsContact, targetMember uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error)
}

type ThreadStore interface {
	Create(ctx context.Context, req *model.Thread) (*model.Thread, error)
	Get(ctx context.Context, query queryobject.QueryObject) (*model.Thread, error)
	Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error)
	ResolveThread(ctx context.Context, q model.ResolveThreadQuery) (*model.Thread, error)
	SearchLeft(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error)
}

type ThreadPermissionStore interface {
	Create(ctx context.Context, in *model.ThreadPermission) (*model.ThreadPermission, error)
	Get(ctx context.Context, in *model.ThreadPermissionStoreFilters) ([]*model.ThreadPermission, error)
	Update(ctx context.Context, in *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error)
}

type MessageHistory interface {
	Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Message, error)
}

type MessageRevisionStore interface {
	Search(ctx context.Context, messageID uuid.UUID, domainID int32, callerID uuid.UUID) ([]*model.MessageChangeEntry, error)
}

type DirectThreadDialogOrchestration interface {
	InitializeFullDirectThread(ctx context.Context, req *model.CreateDirectThreadDialogRequest) ([]*model.DirectThreadDialog, error)
}

type InteractiveCallback interface {
	Save(ctx context.Context, callback *model.InteractiveCallback) (*model.InteractiveCallback, error)
}

// MessageReactionStore persists emoji reactions. A member holds at most one
// reaction per message; SetReaction applies set/replace/remove toggle semantics
// idempotently and reports the resulting state.
type MessageReactionStore interface {
	SetReaction(ctx context.Context, reaction *model.Reaction) (*model.ReactionResult, error)
}

type ThreadVariablesStore interface {
	Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error)
	Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error)
	Locate(ctx context.Context, threadID uuid.UUID) (*model.ThreadVariables, error)
	Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error)
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

	// ClearController sets thread.bot_controller_id to NULL and returns the member id it pointed
	// to (nil if there was none). Used to release a controller that lingers without a matching
	// stack entry (legacy data, or the owner-bot fallback), so /close stays effective.
	ClearController(ctx context.Context, threadID uuid.UUID) (*uuid.UUID, error)
}
