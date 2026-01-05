package model

import (
	"time"

	"github.com/google/uuid"
)

type MessageType string

const (
	MessageTypeText   MessageType = "text"
	MessageTypeFile   MessageType = "file"
	MessageTypeImage  MessageType = "image"
	MessageTypeSystem MessageType = "system"
)

type Message struct {
	Id        uuid.UUID   `json:"id" db:"id"`
	ThreadId  uuid.UUID   `json:"thread_id" db:"thread_id"`
	From      Peer        `json:"from" db:"from_"`
	To        Peer        `json:"to" db:"to_"`
	Text      string      `json:"text" db:"body"`
	Type      MessageType `json:"type" db:"type"`
	Metadata  any         `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt time.Time   `json:"updated_at" db:"updated_at"`
}

type MediaContent struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}
