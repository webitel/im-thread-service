package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	DeleteMessagesRequest struct {
		DeletedBy shared.Peer `json:"deleted_by"`
		IDs       uuid.UUIDs  `json:"ids"`
		DomainID  int32       `json:"domain_id"`
	}

	// DeleteMessagesResponse reports the best-effort outcome: DeletedIDs holds
	// the messages this call removed, Skipped the requested ones left untouched
	// with the reason for each.
	DeleteMessagesResponse struct {
		DeletedIDs uuid.UUIDs          `json:"deleted_ids"`
		Skipped    []model.MessageSkip `json:"skipped"`
		DeletedAt  time.Time           `json:"deleted_at"`
	}
)
