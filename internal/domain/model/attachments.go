package model

import (
	"net/url"
	"time"

	"github.com/google/uuid"
)

type MessageImage struct {
	ID         uuid.UUID      `json:"id" db:"id"`
	MessageID  uuid.UUID      `json:"message_id" db:"message_id"`
	FileID     int64          `json:"file_id" db:"file_id"`
	Name       string         `json:"name" db:"name"`
	Mime       string         `json:"mime" db:"mime"`
	Width      int32          `json:"width,omitempty" db:"width"`
	Height     int32          `json:"height,omitempty" db:"height"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
	Thumbnails map[string]any `json:"thumbnails,omitempty" db:"thumbnails"`
	URL        *url.URL       `json:"url,omitempty"`
}

type MessageDocument struct {
	ID        uuid.UUID `json:"id" db:"id"`
	MessageID uuid.UUID `json:"message_id" db:"message_id"`
	FileID    int64     `json:"file_id" db:"file_id"`
	Name      string    `json:"name" db:"name"`
	Mime      string    `json:"mime" db:"mime"`
	Size      int64     `json:"size" db:"size"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	URL       *url.URL  `json:"url,omitempty"`
}
