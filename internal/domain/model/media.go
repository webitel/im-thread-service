package model

import "net/url"

type Image struct {
	BaseModel
	Thumbnails []*Thumbnail `json:"thumbnails"`
	URL        *url.URL     `json:"url"`
	FileName   string       `json:"file_name"`
	MimeType   string       `json:"mime_type"`
}

type Document struct {
	BaseModel
	URL       *url.URL `json:"url"`
	FileName  string   `json:"file_name"`
	MimeType  string   `json:"mime_type"`
	SizeBytes int64    `json:"size_bytes"`
}

type Thumbnail struct {
	URL *url.URL `json:"url"`
}
