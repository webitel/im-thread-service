package model

import (
	"net/url"
	"time"

	"github.com/google/uuid"
)

//go:generate stringer -type=MessageType
type MessageType int16

const (
	MessageTypeText   MessageType = iota + 1 // 1: TEXT
	MessageTypeFile                          // 2: FILE (DOC)
	MessageTypeImage                         // 3: IMAGE
	MessageTypeSystem                        // 4: SYSTEM
)

type Message struct {
	Id        uuid.UUID      `json:"id" db:"id"`
	ThreadId  uuid.UUID      `json:"thread_id" db:"thread_id"`
	From      Peer           `json:"from" db:"from"`
	To        Peer           `json:"to" db:"to"`
	Text      string         `json:"text" db:"body"`
	Type      MessageType    `json:"type" db:"type"`
	Metadata  map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

type MediaContent struct {
	Id       string   `json:"id"`
	URL      *url.URL `json:"url"`
	MimeType string   `json:"mime_type"`
	Size     int64    `json:"size"`
}

type Entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Value  string `json:"value"`
}
