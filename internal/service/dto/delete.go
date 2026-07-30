package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	DeleteMessagesRequest struct {
		DeletedBy shared.Peer `json:"deleted_by"`
		IDs       uuid.UUIDs  `json:"ids"`
	}

	// DeleteMessagesResponse reports the best-effort outcome: DeletedIDs holds
	// the messages this call removed, SkippedIDs the requested ones left
	// untouched.
	DeleteMessagesResponse struct {
		DeletedIDs uuid.UUIDs `json:"deleted_ids"`
		SkippedIDs uuid.UUIDs `json:"skipped_ids"`
		DeletedAt  time.Time  `json:"deleted_at"`
	}
)
