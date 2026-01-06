package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type (
	Store interface{}

	MessageStore interface {
	}

	ThreadDialogStore interface {
		Resolve(ctx context.Context, search *dto.SearchThreadDialogRequest) (uuid.UUID, error)
		CreateDirectPair(ctx context.Context, dialog *model.ThreadDialog) ([]*model.ThreadDialog, error) //or just return one?
	}

	ThreadStore interface {
		Create(ctx context.Context, req *model.Thread) (*model.Thread, error)
	}
)
