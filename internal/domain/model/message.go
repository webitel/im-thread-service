package model

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

//go:generate stringer -type=MessageType
type MessageType int16

const (
	MessageTypeUnknown     MessageType = iota
	MessageTypeText                    // 1: TEXT
	MessageTypeFile                    // 2: FILE (DOC)
	MessageTypeImage                   // 3: IMAGE
	MessageTypeSystem                  // 4: SYSTEM
	MessageTypeInteractive             // 5: INTERACTIVE
	MessageTypeLocation                // 6: LOCATION
	MessageTypeContact                 // 7: CONTACT
)

type Message struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	IdempotencyKey string          `json:"idempotency_key" db:"-"`
	ThreadID       uuid.UUID       `json:"thread_id" db:"thread_id"`
	DomainID       int32           `json:"domain_id" db:"domain_id"`
	From           shared.Peer     `json:"from" db:"from"`
	SendTo         shared.Peer     `json:"send_to" db:"send_to"`
	To             []*ThreadDialog `json:"to" db:"to"`
	Body           string          `json:"body" db:"body"`
	Type           MessageType     `json:"type" db:"type"`
	Metadata       map[string]any  `json:"metadata,omitempty" db:"metadata"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
	SenderID       uuid.UUID       `json:"sender_id" db:"sender_id"`

	Images      []*MessageImage     `json:"images,omitempty" db:"images"`
	Documents   []*MessageDocument  `json:"documents,omitempty" db:"documents"`
	Location    *MessageLocation    `json:"location,omitempty" db:"location"`
	Contact     *MessageContact     `json:"contact,omitempty" db:"contact"`
	Interactive *MessageInteractive `json:"interactive,omitempty" db:"interactive"`
	Member      *ThreadDialog       `json:"member,omitempty" db:"member"`

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

func (m *Message) WithCreatedEvent(sendID string, from *shared.Peer) *Message {
	if m == nil {
		return m
	}

	to := make([]*event.ThreadMember, 0, len(m.To))
	memberMapper := func(member *ThreadDialog) *event.ThreadMember {
		var memberID *uuid.UUID
		if member.ID != uuid.Nil {
			memberID = &member.ID
		}

		return &event.ThreadMember{
			ID:        memberID,
			ContactID: member.ContactID,
			Role:      int(member.ThreadRole),
		}
	}

	for _, member := range m.To {
		to = append(to, memberMapper(member))
	}

	e := event.MessageCreated{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		From:       from,
		To:         to,
		SendID:     sendID,
		Body:       m.Body,
		Type:       int16(m.Type),
		OccurredAt: m.CreatedAt,
		Metadata:   maps.Clone(m.Metadata),
	}

	if len(m.Images) > 0 {
		e.Images = mapImagesToPayload(m.Images)
	}

	if len(m.Documents) > 0 {
		e.Documents = mapDocumentsToPayload(m.Documents)
	}

	if m.Location != nil {
		e.Location = event.NewLocationPayload(m.Location.Latitude, m.Location.Longitude, m.Location.Address, m.Location.Name)
	}

	if m.Contact != nil {
		e.Contact = event.NewContactPayload(m.Contact.Name, m.Contact.PhoneNumber, m.Contact.Email)
	}

	if m.Interactive != nil {
		var err error
		e.Interactive, err = json.Marshal(m.Interactive)
		if err != nil {
			e.Interactive = nil
		}
	}

	m.AddEvent(e)

	return m
}
