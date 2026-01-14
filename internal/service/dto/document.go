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
	URL      string
}

type DocumentRequest struct {
	Body      string
	Documents []*Document
}

type SendDocumentRequest struct {
	From     model.Peer
	To       model.Peer
	Document DocumentRequest
	DomainID int64 `json:"domain_id"`
}

type SendDocumentResponse struct {
	ID uuid.UUID
	To model.Peer
}

func (d *Document) GetID() int64        { return d.ID }
func (d *Document) GetURL() string      { return d.URL }
func (d *Document) GetMimeType() string { return d.MimeType }
func (d *Document) GetName() string     { return d.Name }
func (d *Document) SetID(id int64)      { d.ID = id }
func (d *Document) SetMime(mime string) { d.MimeType = mime }
func (d *Document) SetName(name string) { d.Name = name }
