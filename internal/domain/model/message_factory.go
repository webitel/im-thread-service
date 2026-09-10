package model

import (
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type ImageInput struct {
	FileID   string
	Name     string
	MimeType string
	URL      string
}

type DocumentInput struct {
	URL      string
	FileID   string
	Name     string
	MimeType string
	Size     int64
}

// MessageCreate groups all necessary data to initialize a new Message domain entity.
// This pattern avoids long positional argument lists and makes the API more extensible.
type MessageCreate struct {
	ThreadID   uuid.UUID
	DomainID   int32
	From       shared.Peer
	MemberID   uuid.UUID
	Recipients []*ThreadDialog
	Body       string
	SendID     string          // Used for client-side correlation in RabbitMQ events.
	Images     []ImageInput    // Data for image attachments.
	Documents  []DocumentInput // Data for file attachments.
	Entities   []shared.Entity // Text formatting entities (bold, italic, etc.).
}

// NewDocumentMessage initializes a message with document attachments and stages events.
func NewDocumentMessage(in MessageCreate) *Message {
	// Caller-supplied entities carry offsets measured against in.Body as received; trimming/NFC
	// normalization would shift those offsets, so skip it when entities are present (mirrors
	// SendText, which never normalizes body text either). The regex fallback path still gets
	// a normalized text to extract from.
	cleanText := in.Body
	if len(in.Entities) == 0 {
		cleanText = prepareText(in.Body)
	}

	domainDocs := make([]*MessageDocument, 0, len(in.Documents))
	for _, d := range in.Documents {
		fID, _ := strconv.ParseInt(d.FileID, 10, 64)
		domainDocs = append(domainDocs, &MessageDocument{
			FileID: fID,
			Name:   d.Name,
			Mime:   d.MimeType,
			Size:   d.Size,
			URL:    d.URL,
		})
	}

	msg := &Message{
		ID:       uuid.New(),
		ThreadID: in.ThreadID,
		DomainID: in.DomainID,
		From:     in.From,
		Member: &ThreadDialog{
			BaseModel: shared.BaseModel{ID: in.MemberID},
		},
		To:        in.Recipients,
		Body:      cleanText,
		Type:      MessageTypeFile,
		Documents: domainDocs,
		Metadata:  BuildMetadata(cleanText, in.Entities),
		CreatedAt: time.Now().UTC(),
	}

	return msg
}

// --- Internal Helpers ---

func mapImagesToPayload(imgs []*MessageImage) []event.ImagePayload {
	res := make([]event.ImagePayload, 0, len(imgs))
	for _, i := range imgs {
		res = append(res, event.ImagePayload{FileID: i.FileID, Mime: i.Mime, Name: i.Name, URL: i.URL})
	}

	return res
}

func mapDocumentsToPayload(docs []*MessageDocument) []event.DocumentPayload {
	res := make([]event.DocumentPayload, 0, len(docs))
	for _, d := range docs {
		res = append(res, event.DocumentPayload{FileID: d.FileID, Mime: d.Mime, Name: d.Name, Size: d.Size, URL: d.URL})
	}

	return res
}
