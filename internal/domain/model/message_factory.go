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
	Recipients uuid.UUIDs
	Body       string
	SendID     string          // Used for client-side correlation in RabbitMQ events.
	Images     []ImageInput    // Data for image attachments.
	Documents  []DocumentInput // Data for file attachments.
}

// NewTextMessage initializes a standard text message and stages events for all recipients.
func NewTextMessage(in MessageCreate) *Message {
	cleanText := prepareText(in.Body)
	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  in.ThreadID,
		DomainID:  in.DomainID,
		From:      in.From,
		To:        in.Recipients,
		Body:      cleanText,
		Type:      MessageTypeText,
		Metadata:  BuildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	addCreatedEvents(msg, in)
	return msg
}

// NewImageMessage initializes a message with image attachments and stages events.
func NewImageMessage(in MessageCreate) *Message {
	cleanText := prepareText(in.Body)
	domainImages := make([]*MessageImage, 0, len(in.Images))
	for _, img := range in.Images {
		fID, _ := strconv.ParseInt(img.FileID, 10, 64)
		domainImages = append(domainImages, &MessageImage{
			FileID: fID,
			Name:   img.Name,
			Mime:   img.MimeType,
			URL:    img.URL,
		})
	}

	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  in.ThreadID,
		DomainID:  in.DomainID,
		From:      in.From,
		To:        in.Recipients,
		Body:      cleanText,
		Type:      MessageTypeImage,
		Images:    domainImages,
		Metadata:  BuildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	return msg
}

// NewDocumentMessage initializes a message with document attachments and stages events.
func NewDocumentMessage(in MessageCreate) *Message {
	cleanText := prepareText(in.Body)
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
		ID:        uuid.New(),
		ThreadID:  in.ThreadID,
		DomainID:  in.DomainID,
		From:      in.From,
		To:        in.Recipients,
		Body:      cleanText,
		Type:      MessageTypeFile,
		Documents: domainDocs,
		Metadata:  BuildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	return msg
}

// --- Internal Helpers ---

// addCreatedEvents is a private helper to avoid code duplication across factories.
// It maps the domain message state to a set of transport-ready events.
func addCreatedEvents(msg *Message, in MessageCreate) {
	ev := event.MessageCreated{
		MessageID:  msg.ID,
		ThreadID:   msg.ThreadID,
		DomainID:   msg.DomainID,
		SendID:     in.SendID, // Passed through only to the event
		From:       &shared.Peer{ID: msg.From.ID, Type: msg.From.Type},
		To:         in.Recipients,
		Body:       msg.Body,
		Type:       int16(msg.Type),
		OccurredAt: msg.CreatedAt,
	}
	// Attach media payloads if present
	if len(msg.Images) > 0 {
		ev.Images = mapImagesToPayload(msg.Images)
	}
	if len(msg.Documents) > 0 {
		ev.Documents = mapDocumentsToPayload(msg.Documents)
	}
	msg.AddEvent(ev)
}

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
