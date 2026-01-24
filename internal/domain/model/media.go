package model

import (
	"net/url"

	"github.com/google/uuid"
)

type Image struct {
	BaseModel
	MessageId  uuid.UUID
	Thumbnails []*Thumbnail `json:"thumbnails"`
	URL        *url.URL     `json:"url"`
	FileName   string       `json:"file_name"`
	MimeType   string       `json:"mime_type"`
}

type Document struct {
	BaseModel
	MessageId uuid.UUID
	URL       *url.URL `json:"url"`
	FileName  string   `json:"file_name"`
	MimeType  string   `json:"mime_type"`
	SizeBytes int64    `json:"size_bytes"`
}

type Thumbnail struct {
	URL *url.URL `json:"url"`
}
