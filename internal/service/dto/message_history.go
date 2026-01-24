package dto

import (
	"time"

	"github.com/google/uuid"
)

type (
	HistoryDocument struct {
		ID        uuid.UUID `json:"id"`
		MessageID uuid.UUID `json:"message_id"`
		FileID    int       `json:"file_id"`
		Name      string    `json:"name"`
		Mime      string    `json:"mime"`
		Size      int64     `json:"size"`
		CreatedAt time.Time `json:"created_at"`
	}

	HistoryImage struct {
		ID        uuid.UUID `json:"id"`
		MessageID uuid.UUID `json:"message_id"`
		FileID    int       `json:"file_id"`
		Mime      string    `json:"mime"`
		Width     int       `json:"width"`
		Height    int       `json:"height"`
		CreatedAt time.Time `json:"created_at"`
	}

	HistoryMessage struct {
		ID         uuid.UUID
		ThreadID   uuid.UUID
		SenderID   uuid.UUID
		ReceiverID uuid.UUID
		Type       int
		Body       string
		Metadata   map[string]any
		CreatedAt  time.Time
		UpdatedAt  time.Time

		Documents []HistoryDocument
		Images    []HistoryImage
	}

	HistoryMessageInputDTO struct {
		Fields      []string
		Ids         uuid.UUIDs
		ThreadIds   uuid.UUIDs
		SenderIds   uuid.UUIDs
		ReceiverIds uuid.UUIDs
		Types       []int
		Cursor      *HistoryMessageCursor
		Sort        string
		Size        int
	}

	HistoryMessageCursor struct {
		CreatedAt time.Time
		Id        uuid.UUID
		Direction bool
	}
)
