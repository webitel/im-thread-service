package model

import (
	"net/url"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type Image struct {
	shared.BaseModel

	Thumbnails []*Thumbnail `json:"thumbnails"`
	URL        *url.URL     `json:"url"`
	FileName   string       `json:"file_name"`
	MimeType   string       `json:"mime_type"`
}

type Document struct {
	shared.BaseModel

	URL       *url.URL `json:"url"`
	FileName  string   `json:"file_name"`
	MimeType  string   `json:"mime_type"`
	SizeBytes int64    `json:"size_bytes"`
}

type Thumbnail struct {
	URL *url.URL `json:"url"`
}
