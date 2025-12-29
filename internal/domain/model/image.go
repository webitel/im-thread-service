package model

import "net/url"

type Image struct {
	BaseModel

	Thumbnails []*Thumbnail `json:"thumbnails"`
	Url        *url.URL     `json:"url"`
	FileName   string       `json:"file_name"`
	MimeType   string       `json:"mime_type"`
}

type Thumbnail struct {
	Url *url.URL `json:"url"`
}
