package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type (
	Store interface {
		Messages() MessageStore
		Outbox() OutboxStore
		ThreadDialog() ThreadDialogStore
		Thread() ThreadStore
	}

	MessageStore interface {
		SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
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
)
