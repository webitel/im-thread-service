package dto

import (
	"net/url"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type Document struct {
	ID       int64
	Name     string
	MimeType string
	Size     int64
	URL      string
}

func (d *Document) Validate() error {
	if d == nil {
		return errors.InvalidArgument("document is required", errors.WithID("dto.document.validate"))
	}

	if d.ID <= 0 && d.URL == "" {
		return errors.InvalidArgument("either ID or URL is reqired", errors.WithID("dto.document.validate"))
	}

	if d.URL != "" {
		if _, err := url.Parse(d.URL); err != nil {
			return errors.InvalidArgument("provided invalid download URL", errors.WithCause(err), errors.WithID("dto.document.validate"))
		}
	}

	return nil
}

type DocumentRequest struct {
	Body      string
	Documents []*Document
}

func (documentRequest *DocumentRequest) Validate() error {
	if documentRequest == nil {
		return errors.InvalidArgument("document request is required", errors.WithID("dto.document.validate"))
	}

	for i := range documentRequest.Documents {
		if err := documentRequest.Documents[i].Validate(); err != nil {
			return err
		}
	}

	return nil
}

type SendDocumentRequest struct {
	From     shared.Peer
	To       shared.Peer
	Document DocumentRequest
	DomainID int64  `json:"domain_id"`
	SendID   string `json:"send_id"`
	SendAs   *uuid.UUID

	ReplyToMessageID  *uuid.UUID `json:"reply_to_message_id,omitempty"`
	ExternalID        string     `json:"external_id,omitempty"`
	ReplyToExternalID string     `json:"reply_to_external_id,omitempty"`
}

func (sendDocumentRequest *SendDocumentRequest) Validate() error {
	if sendDocumentRequest == nil {
		return errors.InvalidArgument("send document request is required", errors.WithID("dto.document.validate"))
	}

	if sendDocumentRequest.DomainID <= 0 {
		return errors.InvalidArgument("domain id is required and must be gt 0", errors.WithID("dto.document.validate"))
	}

	if sendDocumentRequest.From.ID == uuid.Nil {
		return errors.InvalidArgument("from peer id is required", errors.WithID("dto.document.validate"))
	}

	if sendDocumentRequest.To.ID == uuid.Nil {
		return errors.InvalidArgument("to peer is required", errors.WithID("dto.document.validate"))
	}

	if sendDocumentRequest.ReplyToMessageID != nil && *sendDocumentRequest.ReplyToMessageID == uuid.Nil {
		return errors.InvalidArgument("reply_to_message_id is not a valid uuid", errors.WithID("dto.document.validate"))
	}

	return nil
}

type SendDocumentResponse struct {
	ID uuid.UUID
	To shared.Peer
}

func (d *Document) GetID() int64        { return d.ID }
func (d *Document) GetURL() string      { return d.URL }
func (d *Document) GetMimeType() string { return d.MimeType }
func (d *Document) GetName() string     { return d.Name }
func (d *Document) GetSize() int64      { return d.Size }
func (d *Document) SetID(id int64)      { d.ID = id }
func (d *Document) SetMime(mime string) { d.MimeType = mime }
func (d *Document) SetName(name string) { d.Name = name }
func (d *Document) SetURL(url string)   { d.URL = url }
func (d *Document) SetSize(size int64)  { d.Size = size }
