package dto

import (
	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

const MaxForwardBatch = 20

type (
	ForwardMessagesRequest struct {
		From       shared.Peer `json:"from"`
		To         shared.Peer `json:"to"`
		DomainID   int64       `json:"domain_id"`
		SendID     string      `json:"send_id"`
		SendAs     *uuid.UUID
		MessageIDs []uuid.UUID `json:"message_ids"`
	}

	ForwardMessagesResponse struct {
		To         shared.Peer `json:"to"`
		ThreadID   uuid.UUID   `json:"thread_id"`
		IDs        []uuid.UUID `json:"ids"`
		SkippedIDs []uuid.UUID `json:"skipped_ids"`
	}
)
