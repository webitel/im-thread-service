package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	Image struct {
		ID       int64  `json:"id"`
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		Name     string `json:"name"`
	}

	ImageRequest struct {
		Images []*Image `json:"images"`
		Body   string   `json:"body"`
	}

	SendImageRequest struct {
		From     shared.Peer  `json:"from"`
		To       shared.Peer  `json:"to"`
		Image    ImageRequest `json:"image"`
		DomainID int64        `json:"domain_id"`
	}

	SendImageResponse struct {
		To shared.Peer `json:"to"`
		ID uuid.UUID   `json:"id"`
	}
)

func (i *Image) GetID() int64        { return i.ID }
func (i *Image) GetURL() string      { return i.URL }
func (i *Image) GetMimeType() string { return i.MimeType }
func (i *Image) GetName() string     { return i.Name }
func (i *Image) SetID(id int64)      { i.ID = id }
func (i *Image) SetMime(mime string) { i.MimeType = mime }
func (i *Image) SetName(name string) { i.Name = name }
func (i *Image) SetURL(url string)   { i.URL = url }
