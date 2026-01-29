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
	MimeType string
	Name     string
}

type DocumentInput struct {
	FileID   string
	MimeType string
	Name     string
	Size     int64
}

func NewTextMessage(threadID uuid.UUID, domainID int32, from shared.Peer, recipients []shared.Peer, text string) *Message {
	cleanText := prepareText(text)
	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  threadID,
		DomainID:  domainID,
		From:      from,
		To:        recipients[0],
		Text:      cleanText,
		Type:      MessageTypeText,
		Metadata:  buildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	for _, to := range recipients {
		msg.AddEvent(event.MessageCreated{
			DomainID:  domainID,
			MessageID: msg.ID,
			ThreadID:  msg.ThreadID,
			From: &shared.Peer{
				ID:   msg.From.ID,
				Type: msg.From.Type,
			},
			To: &shared.Peer{
				ID:   to.ID,
				Type: to.Type,
			},
			Body:       msg.Text,
			Type:       int16(msg.Type),
			OccurredAt: msg.CreatedAt,
		})
	}
	return msg
}

func NewImageMessage(threadID uuid.UUID, domainID int32, from shared.Peer, recipients []shared.Peer, text string, images []ImageInput) *Message {
	cleanText := prepareText(text)
	domainImages := make([]*MessageImage, 0, len(images))
	for _, img := range images {
		fID, _ := strconv.ParseInt(img.FileID, 10, 64)
		domainImages = append(domainImages, &MessageImage{
			FileID: fID,
			Mime:   img.MimeType,
			Name:   img.Name,
		})
	}

	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  threadID,
		DomainID:  domainID,
		From:      from,
		To:        recipients[0],
		Text:      cleanText,
		Type:      MessageTypeImage,
		Images:    domainImages,
		Metadata:  buildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	for _, to := range recipients {
		msg.AddEvent(event.MessageCreated{
			MessageID: msg.ID,
			ThreadID:  msg.ThreadID,
			DomainID:  domainID,
			From: &shared.Peer{
				ID:   msg.From.ID,
				Type: msg.From.Type,
			},
			To: &shared.Peer{
				ID:   to.ID,
				Type: to.Type,
			},
			Body:       msg.Text,
			Type:       int16(msg.Type),
			Images:     mapImagesToPayload(msg.Images),
			OccurredAt: msg.CreatedAt,
		})
	}
	return msg
}

func NewDocumentMessage(threadID uuid.UUID, domainID int32, from shared.Peer, recipients []shared.Peer, body string, docs []DocumentInput) *Message {
	cleanText := prepareText(body)
	domainDocs := make([]*MessageDocument, 0, len(docs))
	for _, d := range docs {
		fID, _ := strconv.ParseInt(d.FileID, 10, 64)
		domainDocs = append(domainDocs, &MessageDocument{
			FileID: fID,
			Name:   d.Name,
			Mime:   d.MimeType,
			Size:   d.Size,
		})
	}

	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  threadID,
		DomainID:  domainID,
		From:      from,
		To:        recipients[0],
		Text:      cleanText,
		Type:      MessageTypeFile,
		Documents: domainDocs,
		Metadata:  buildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	for _, to := range recipients {
		msg.AddEvent(event.MessageCreated{
			MessageID: msg.ID,
			ThreadID:  msg.ThreadID,
			DomainID:  domainID,
			From: &shared.Peer{
				ID:   msg.From.ID,
				Type: msg.From.Type,
			},
			To: &shared.Peer{
				ID:   to.ID,
				Type: to.Type,
			},
			Body:       msg.Text,
			Type:       int16(msg.Type),
			Documents:  mapDocumentsToPayload(msg.Documents),
			OccurredAt: msg.CreatedAt,
		})
	}
	return msg
}

// --- Internal Payloads Mappers ---

func mapImagesToPayload(imgs []*MessageImage) []event.ImagePayload {
	res := make([]event.ImagePayload, 0, len(imgs))
	for _, i := range imgs {
		res = append(res, event.ImagePayload{FileID: i.FileID, Mime: i.Mime, Name: i.Name})
	}
	return res
}

func mapDocumentsToPayload(docs []*MessageDocument) []event.DocumentPayload {
	res := make([]event.DocumentPayload, 0, len(docs))
	for _, d := range docs {
		res = append(res, event.DocumentPayload{FileID: d.FileID, Mime: d.Mime, Name: d.Name, Size: d.Size})
	}
	return res
}
