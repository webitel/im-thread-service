package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	SendTextRequest struct {
		From model.Peer `json:"from"`
		To   model.Peer `json:"to"`
		Body string     `json:"body"`
	}

	SendTextResponse struct {
		To model.Peer `json:"to"`
		ID uuid.UUID  `json:"id"`
	}
)
