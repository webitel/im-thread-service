package store

import (
	"context"

	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	Store interface {
		WithTx(ctx context.Context, fn func(ctx context.Context) error) error
		Messages() MessageStore
		Outbox() OutboxStore
	}

	MessageStore interface {
		SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error)
	}

	OutboxStore interface {
		Publish(ctx context.Context, topic string, event events.Outboxer) error
		Cleanup(ctx context.Context, retentionDays int) (int64, error)
	}
)
