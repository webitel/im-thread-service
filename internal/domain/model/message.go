package model

import (
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
	ID             uuid.UUID `json:"id" db:"id"`
	SendAs         *uuid.UUID
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
	MemberID       uuid.UUID       `json:"member_id" db:"member_id"`

	Images      []*MessageImage     `json:"images,omitempty" db:"images"`
	Documents   []*MessageDocument  `json:"documents,omitempty" db:"documents"`
	Location    *MessageLocation    `json:"location,omitempty" db:"location"`
	Contact     *MessageContact     `json:"contact,omitempty" db:"contact"`
	Interactive *MessageInteractive `json:"interactive,omitempty" db:"interactive"`
	System      *MessageSystem      `json:"system,omitempty" db:"system"`
	Member      *ThreadDialog       `json:"member,omitempty" db:"member"`

	domainEvents []event.Outboxer
}

func (m *Message) GetSender() uuid.UUID {
	if m.SendAs != nil && *m.SendAs != uuid.Nil {
		return *m.SendAs
	}

	return m.From.ID
}

func (m *Message) GetOriginSender() *uuid.UUID {
	if m.SendAs != nil {
		originRef := m.From.ID

		return &originRef
	}

	return nil
}

func (m *Message) SetMemberFromSlice(members []*ThreadDialog) *Message {
	senderID := m.GetSender()
	for _, member := range members {
		if senderID != member.ContactID {
			continue
		}

		m.MemberID = member.ID
		m.Member = &ThreadDialog{
			BaseModel: shared.BaseModel{ID: member.ID},
		}

		break
	}

	return m
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

func (m *Message) WithCreatedEvent(sendID string, _ *shared.Peer) *Message {
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

	var messageFrom *event.ThreadMember

	for _, member := range m.To {
		if member.ContactID == m.From.ID {
			messageFrom = &event.ThreadMember{
				ID:        &member.ID,
				ContactID: member.ContactID,
				Role:      int(member.ThreadRole),
			}
		}
	}

	e := event.MessageCreated{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		From:       messageFrom,
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

	if m.System != nil {
		e.System = event.NewSystemPayload(m.System.Type, m.System.Metadata)
	}

	if m.Interactive != nil {
		e.Interactive = m.Interactive.AsEvent()
	}

	m.AddEvent(e)

	return m
}

func (m *MessageInteractive) AsEvent() *event.InteractivePayload {
	if m == nil {
		return nil
	}

	payload := &event.InteractivePayload{}

	if m.Kind.Markup != nil {
		payload.Markup = mapMarkupToEvent(m.Kind.Markup)
	}

	if m.Kind.ListReply != nil {
		payload.ListReply = mapListReplyToEvent(m.Kind.ListReply)
	}

	return payload
}

// Внутрішні хелпери для мапінгу

func mapMarkupToEvent(markup *KeyboardButtonMarkup) *event.KeyboardMarkup {
	if markup == nil {
		return nil
	}

	rows := make([]*event.KeyboardRow, 0, len(markup.Rows))
	for _, row := range markup.Rows {
		if row == nil {
			continue
		}

		rows = append(rows, &event.KeyboardRow{
			Buttons: mapButtonsToEvent(row.Buttons),
		})
	}

	return &event.KeyboardMarkup{Rows: rows}
}

func mapListReplyToEvent(list *KeyboardListReply) *event.KeyboardListReply {
	if list == nil {
		return nil
	}

	sections := make([]*event.KeyboardRowWithSection, 0, len(list.Sections))
	for _, s := range list.Sections {
		if s == nil {
			continue
		}

		sections = append(sections, &event.KeyboardRowWithSection{
			Section: s.Section,
			Buttons: mapButtonsToEvent(s.Buttons),
		})
	}

	return &event.KeyboardListReply{
		MainButtonTitle: list.Title,
		Sections:        sections,
	}
}

func mapButtonsToEvent(btns []*KeyboardButton) []*event.KeyboardButton {
	res := make([]*event.KeyboardButton, 0, len(btns))
	for _, b := range btns {
		if b == nil {
			continue
		}

		eventBtn := &event.KeyboardButton{
			ID:       b.ID,
			Label:    b.Label,
			Metadata: b.Metadata,
			Type:     b.Type,
		}

		// Мапінг поліморфних полів залежно від типу
		switch b.Type {
		case ActionTypeURL:
			if b.URL != nil {
				eventBtn.URL = &event.KeyboardButtonURL{URL: *b.URL}
			}
		case ActionTypeCallback:
			if b.Data != nil {
				eventBtn.Callback = &event.KeyboardButtonCallback{Data: *b.Data}
			}
		case ActionTypeRequest:
			if b.Action != nil {
				eventBtn.Request = &event.KeyboardButtonRequest{Action: *b.Action}
			}
		}

		res = append(res, eventBtn)
	}

	return res
}
