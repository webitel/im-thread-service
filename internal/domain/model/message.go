package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

//go:generate stringer -type=MessageType
type MessageType int16

const (
	MessageTypeText   MessageType = iota + 1 // 1: TEXT
	MessageTypeFile                          // 2: FILE (DOC)
	MessageTypeImage                         // 3: IMAGE
	MessageTypeSystem                        // 4: SYSTEM
)

type Message struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	ThreadID  uuid.UUID      `json:"thread_id" db:"thread_id"`
	DomainID  int32          `json:"domain_id" db:"domain_id"`
	From      shared.Peer    `json:"from" db:"from"`
	To        uuid.UUIDs     `json:"to" db:"to"`
	Body      string         `json:"body" db:"body"`
	Type      MessageType    `json:"type" db:"type"`
	Metadata  map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
	SenderID  uuid.UUID      `json:"sender_id" db:"sender_id"`

	Images    []*MessageImage    `json:"images,omitempty" db:"images"`
	Documents []*MessageDocument `json:"documents,omitempty" db:"documents"`

	// [TYPED_QUEUE] Strictly limited to outbox-compatible events
	domainEvents []event.Outboxer
}

func (m *Message) CreatedAtUnixMillis() int64 {
	return max(m.CreatedAt.UTC().UnixMilli(), 0)
}

func (m *Message) UpdatedAtUnixMillis() int64 {
	return max(m.UpdatedAt.UTC().UnixMilli(), 0)
}

// AddEvent ensures only valid outboxers are added to the message
func (m *Message) AddEvent(event event.Outboxer) {
	m.domainEvents = append(m.domainEvents, event)
}

// Events returns the staged events for the service layer
func (m *Message) Events() []event.Outboxer {
	e := m.domainEvents
	m.domainEvents = nil
	return e
}

func (m *Message) WithCreatedEvent(sendID string) *Message {
	if m == nil {
		return m
	}

	e := event.MessageCreated{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		From:       &m.From,
		To:         m.To,
		SendID:     sendID,
		Body:       m.Body,
		Type:       int16(m.Type),
		OccurredAt: m.CreatedAt,
	}

	if len(m.Images) > 0 {
		e.Images = mapImagesToPayload(m.Images)
	}

	if len(m.Documents) > 0 {
		e.Documents = mapDocumentsToPayload(m.Documents)
	}

	m.AddEvent(e)

	return m
}
