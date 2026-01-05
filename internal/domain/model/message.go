package model

import (
	"net/url"
	"time"

	"github.com/google/uuid"
)

//go:generate stringer -type=MessageType
type MessageType int16 // smallint у Postgres

const (
	MessageTypeText MessageType = iota
	MessageTypeFile
	MessageTypeImage
	MessageTypeSystem
)

type Message struct {
	Id        uuid.UUID   `json:"id" db:"id"`
	ThreadId  uuid.UUID   `json:"thread_id" db:"thread_id"`
	From      Peer        `json:"from" db:"from"`
	To        Peer        `json:"to" db:"to"`
	Text      string      `json:"text" db:"body"`
	Type      MessageType `json:"type" db:"type"`
	Metadata  any         `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt time.Time   `json:"updated_at" db:"updated_at"`
}

type MediaContent struct {
	Id       string   `json:"id"`
	URL      *url.URL `json:"url"`
	MimeType string   `json:"mime_type"`
	Size     int64    `json:"size"`
}
