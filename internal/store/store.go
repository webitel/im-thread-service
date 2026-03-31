package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type (
	Store interface {
		Messages() MessageStore
		Outbox() OutboxStore
		ThreadDialog() ThreadDialogStore
		Thread() ThreadStore
	}

	MessageStore interface {
		// [MESSAGE] Persistence
		SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
		// [ATTACHMENTS] Persistence
		SaveImages(ctx context.Context, messageID uuid.UUID, images []*model.MessageImage) ([]*model.MessageImage, error)
		SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error)
		ReadMessage(ctx context.Context, read struct {
			DomainID  int32
			ThreadID  uuid.UUID
			MessageID uuid.UUID
			UserID    uuid.UUID
		}) error
	}

	OutboxStore interface {
		Publish(ctx context.Context, topic string, event event.Outboxer) error
		Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error)
	}

	ThreadDialogStore interface {
		Create(ctx context.Context, threadDialog *model.ThreadDialog) (*model.ThreadDialog, error)
		Delete(ctx context.Context, threadID, memberID uuid.UUID) error
		GetQuickView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error)
		GetFullView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error)
	}

	ThreadStore interface {
		Create(ctx context.Context, req *model.Thread) (*model.Thread, error)
		Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error)
		// ResolveDirect find a direct between two peers. Returns thread ID.
		ResolveDirect(ctx context.Context, from, to uuid.UUID) (*model.Thread, error)
	}

	ThreadPermissionStore interface {
		Get(ctx context.Context, in *model.GetThreadPermissionRequest) (*model.ThreadPermission, error)
		Update(ctx context.Context, in *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error)
	}

	MessageHistory interface {
		Search(ctx context.Context, query queryobject.QueryObject) ([]*dto.HistoryMessage, error)
	}

	DirectThreadDialogOrchestration interface {
		InitializeFullDirectThread(ctx context.Context, req *model.CreateDirectThreadDialogRequest) ([]*model.DirectThreadDialog, error)
	}
)
