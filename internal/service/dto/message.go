package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type SendMessageResponse struct {
	ID   uuid.UUID    `json:"id"`
	From *shared.Peer `json:"from"`
	To   uuid.UUIDs   `json:"to"`
}

func NewSendMessageResponse(id uuid.UUID, from *shared.Peer, to ...uuid.UUID) *SendMessageResponse {
	return &SendMessageResponse{
		ID:   id,
		From: from,
		To:   to,
	}
}

func CreateSendMessageResponse(id uuid.UUID, from *shared.Peer, to ...uuid.UUID) SendMessageResponse {
	return SendMessageResponse{
		ID:   id,
		From: from,
		To:   to,
	}
}
