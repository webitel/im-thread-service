package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	SendTextRequest struct {
		From     shared.Peer `json:"from"`
		To       shared.Peer `json:"to"`
		Body     string      `json:"body"`
		DomainID int64       `json:"domain_id"`
	}

	SendTextResponse struct {
		To shared.Peer `json:"to"`
		ID uuid.UUID   `json:"id"`
	}
)
