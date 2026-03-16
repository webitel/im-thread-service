package model

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type Validator interface {
	Validate() error
}

type ThreadTarget interface {
    GetDomainID() int
    GetFrom() shared.Peer
    GetSendTo() shared.Peer
    SetThread(id uuid.UUID, members uuid.UUIDs)
}

//go:generate stringer -type=MessageType
type MessageType int16

const (
	MessageTypeUnknown MessageType = iota //WARNING: used only in down migrations to update check_message_type constraint 
	MessageTypeText   						 // 1: TEXT
	MessageTypeFile                          // 2: FILE (DOC)
	MessageTypeImage                         // 3: IMAGE
	MessageTypeSystem                        // 4: SYSTEM
	MessageTypeInteractive
)

type Message struct {
	ID        uuid.UUID      `json:"id" db:"id" fieldtag:"ign"`
	ThreadID  uuid.UUID      `json:"thread_id" db:"thread_id"`
	DomainID  int32          `json:"domain_id" db:"domain_id"`
	From      shared.Peer    `json:"from" db:"sender_id"`
	To        uuid.UUIDs     `json:"to" db:"-"`
	SendTo shared.Peer     `json:"-" db:"-"`
	Text      string         `json:"text" db:"body"`
	Type      MessageType    `json:"type" db:"type"`
	Metadata  map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time      `json:"created_at" db:"created_at" fieldtag:"ign"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at" fieldtag:"ign"`

	Images         []*MessageImage    `json:"images,omitempty" db:"-"`
	Documents      []*MessageDocument `json:"documents,omitempty" db:"-"`
	MessageButtons MessageButtonsMatrix  `json:"message_buttons" db:"buttons"`

	domainEvents []event.Outboxer
}

func (m *Message) Validate() error {
	if m == nil {
		return errors.InvalidArgument("nil message")
	}

	if m.DomainID <= 0 {
		return errors.InvalidArgument("invalid domain")
	}

	if err := m.MessageButtons.Validate(); err != nil {
		return err
	}

	return nil
}

func (m *Message) AddEvent(event event.Outboxer) {
	m.domainEvents = append(m.domainEvents, event)
}

func (m *Message) Events() []event.Outboxer {
	e := m.domainEvents
	m.domainEvents = nil
	return e
}


func (m *Message) WithCreatedEvent() *Message {
	e := event.MessageCreated{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		From:       &m.From,
		To:         m.To,
		SendID:     uuid.New().String(),
		Body:       m.Text,
		Type:       int16(m.Type),
		OccurredAt: m.CreatedAt,
	}

	if len(m.Images) > 0 {
		e.Images = mapImagesToPayload(m.Images)
	}

	if len(m.Documents) > 0 {
		e.Documents = mapDocumentsToPayload(m.Documents)
	}

	if len(m.MessageButtons) > 0 {
		btns, err := json.Marshal(m.MessageButtons)
		if err != nil {
			//TODO: add internal logger
			slog.Error("error parsing buttons", slog.Any("error", err))
		} else {
			e.Buttons = btns
		}
	}
	
	m.domainEvents = append(m.domainEvents, e)

	return m
}

func (m *Message) GetDomainID() int         { return int(m.DomainID) }
func (m *Message) GetFrom() shared.Peer     { return m.From }
func (m *Message) GetSendTo() shared.Peer   { return m.SendTo }
func (m *Message) SetThread(id uuid.UUID, members uuid.UUIDs) {
    m.ThreadID = id
    m.To = members
}