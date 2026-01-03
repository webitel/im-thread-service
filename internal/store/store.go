package store

import (
	"context"

	"github.com/google/uuid"
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
		Add(ctx context.Context, record OutboxRecord) error
		Cleanup(ctx context.Context, limit int) (int64, error)
		MarkAsPublished(ctx context.Context, id string) error
	}
)

type OutboxRecord struct {
	ID       uuid.UUID
	Topic    string
	Payload  []byte
	Metadata map[string]string
}
