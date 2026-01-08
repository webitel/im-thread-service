package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type Document struct {
	ID       int64
	Name     string
	MimeType string
	Size     int64
}

type DocumentRequest struct {
	Body      string
	Documents []*Document
}

type SendDocumentRequest struct {
	From     model.Peer
	To       model.Peer
	Document DocumentRequest
}

type SendDocumentResponse struct {
	ID uuid.UUID
	To model.Peer
}
