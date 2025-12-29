package model

import "net/url"

type Document struct {
	BaseModel

	Url       *url.URL `json:"url"`
	FileName  string   `json:"file_name"`
	MimeType  string   `json:"mime_type"`
	SizeBytes int64    `json:"size_bytes"`
}
