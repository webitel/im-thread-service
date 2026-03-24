package model

import (
	"encoding/json"
	"maps"
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
	MessageTypeText                       // 1: TEXT
	MessageTypeFile                       // 2: FILE (DOC)
	MessageTypeImage                      // 3: IMAGE
	MessageTypeSystem                     // 4: SYSTEM
	MessageTypeInteractive
	MessageTypeLocation
	MessageTypeContact
)

type Message struct {
	ID        uuid.UUID      `json:"id" db:"id" fieldtag:"ign"`
	ThreadID  uuid.UUID      `json:"thread_id" db:"thread_id"`
	DomainID  int32          `json:"domain_id" db:"domain_id"`
	From      shared.Peer    `json:"from" db:"sender_id"`
	To        uuid.UUIDs     `json:"to" db:"-"`
	SendTo    shared.Peer    `json:"-" db:"-"`
	Text      string         `json:"text" db:"body"`
	Type      MessageType    `json:"type" db:"type"`
	Metadata  map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time      `json:"created_at" db:"created_at" fieldtag:"ign"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at" fieldtag:"ign"`

	Images         []*MessageImage      `json:"images,omitempty" db:"-"`
	Documents      []*MessageDocument   `json:"documents,omitempty" db:"-"`
	Location       *Location            `json:"location" db:"-" fieldtag:"ign"`
	Contact        *Contact             `json:"contact" db:"-" fieldtag:"ign"`
	Interactive    *Interactive         `json:"interactive" db:"interactive"`

	domainEvents []event.Outboxer
}

func (m *Message) Validate() error {
	if m == nil {
		return errors.InvalidArgument("nil message")
	}

	if m.DomainID <= 0 {
		return errors.InvalidArgument("invalid domain")
	}

	if m.Contact != nil {
		if err := m.Contact.Validate(); err != nil {
			return err
		}
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

	if m.Interactive != nil {
		interactivePart, err := json.Marshal(m.Interactive)
		if err == nil {
			e.Interactive = interactivePart
		}
	}

	if m.Location != nil {
		e.Location = &event.LocationPayload{
			Address:   m.Location.Address,
			Latitude:  m.Location.Latitude,
			Longitude: m.Location.Longitude,
			Name:      m.Location.Name,
		}
	}

	if m.Contact != nil {
		e.Contact = &event.ContactPayload{
			Name:        m.Contact.Name,
			Email:       m.Contact.Email,
			PhoneNumber: m.Contact.PhoneNumber,
			Metadata:    maps.Clone(m.Contact.Metadata),
		}
	}

	m.domainEvents = append(m.domainEvents, e)

	return m
}

func (m *Message) GetDomainID() int       { return int(m.DomainID) }
func (m *Message) GetFrom() shared.Peer   { return m.From }
func (m *Message) GetSendTo() shared.Peer { return m.SendTo }
func (m *Message) SetThread(id uuid.UUID, members uuid.UUIDs) {
	m.ThreadID = id
	m.To = members
}
