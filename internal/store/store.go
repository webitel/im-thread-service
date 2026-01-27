package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/events"
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
	}

	OutboxStore interface {
		Publish(ctx context.Context, topic string, event events.Outboxer) error
		Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error)
	}

	ThreadDialogStore interface {
		Resolve(ctx context.Context, search *dto.SearchThreadDialogRequest) (uuid.UUID, error)
		CreateDirectPair(ctx context.Context, dialog *model.ThreadDialog) ([]*model.ThreadDialog, error) // or just return one?
	}

	ThreadStore interface {
		Create(ctx context.Context, req *model.Thread) (*model.Thread, error)
	}

	MessageHistory interface {
		Search(ctx context.Context, query queryobject.QueryObject) ([]*dto.HistoryMessage, error)
	}

	DirectSettings interface {
		Create(ctx context.Context, setting *model.DirectThreadSetting) (*model.DirectThreadSetting, error)
		Search(ctx context.Context, query queryobject.QueryObject) ([]*model.DirectThreadSetting, error)

		//NOT IMPLEMENTED!
		Update(ctx context.Context) ([]*model.DirectThreadSetting, error)
		//NOT IMPLEMENTED!
		Delete(ctx context.Context) error
	}

	DirectThreadDialogOrchestration interface {
		InitializeFullDirectThread(ctx context.Context, directThread *model.DirectThreadDialog) ([]*model.DirectThreadDialog, error)
	}
)
