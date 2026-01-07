package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	Image struct {
		ID       string `json:"id"`
		Link     string `json:"link"`
		MimeType string `json:"mime_type"`
	}

	ImageRequest struct {
		Images []*Image `json:"images"`
		Body   string   `json:"body"`
	}

	SendImageRequest struct {
		From  model.Peer   `json:"from"`
		To    model.Peer   `json:"to"`
		Image ImageRequest `json:"image"`
	}

	SendImageResponse struct {
		To model.Peer `json:"to"`
		ID uuid.UUID  `json:"id"`
	}
)
