package model

import (
	"context"
	"maps"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

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

var messageTypeNames = map[MessageType]string{
	MessageTypeText:        "text",
	MessageTypeFile:        "document",
	MessageTypeImage:       "image",
	MessageTypeSystem:      "system",
	MessageTypeInteractive: "interactive",
	MessageTypeLocation:    "location",
	MessageTypeContact:     "contact",
}

func (t MessageType) String() string {
	if s, ok := messageTypeNames[t]; ok {
		return s
	}

	return "text"
}

type Message struct {
	ID             uuid.UUID `json:"id" db:"id"`
	SendAs         *uuid.UUID
	IdempotencyKey string `json:"idempotency_key" db:"-"`

	ExternalID string `json:"external_id,omitempty" db:"-"`

	ReplyToExternalID string          `json:"reply_to_external_id,omitempty" db:"-"`
	ThreadID          uuid.UUID       `json:"thread_id" db:"thread_id"`
	DomainID          int32           `json:"domain_id" db:"domain_id"`
	From              shared.Peer     `json:"from" db:"from"`
	SendTo            shared.Peer     `json:"send_to" db:"send_to"`
	To                []*ThreadDialog `json:"to" db:"to"`
	Body              string          `json:"body" db:"body"`
	Type              MessageType     `json:"type" db:"type"`
	Metadata          map[string]any  `json:"metadata,omitempty" db:"metadata"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
	Edited            bool            `json:"edited" db:"edited"`
	SenderID          uuid.UUID       `json:"sender_id" db:"sender_id"`
	MemberID          uuid.UUID       `json:"member_id" db:"member_id"`

	DeletedAt *time.Time    `json:"deleted_at,omitempty" db:"deleted_at"`
	DeletedBy *ThreadDialog `json:"deleted_by,omitempty" db:"deleted_by"`

	// SkipReason is set by DeleteMessages only: unspecified on a message the
	// call deleted, otherwise why it was left untouched.
	SkipReason MessageSkipReason `json:"-" db:"reason"`

	// RevisionCount is how many entries the message has in its change history.
	// Zero means it was never edited or deleted.
	RevisionCount int32 `json:"revision_count" db:"revision_count"`

	// Version is the position of the live body in the message's change history,
	// the number GetMessageRevisions reports for it: 2 right after the first
	// edit, since version 1 is the original text. Set by EditMessage only.
	Version int32 `json:"version" db:"version"`

	// Seq is the per-thread monotonic sequence number, assigned on message creation.
	Seq int64 `json:"seq" db:"seq"`

	Images          []*MessageImage      `json:"images,omitempty" db:"images"`
	Documents       []*MessageDocument   `json:"documents,omitempty" db:"documents"`
	Location        *MessageLocation     `json:"location,omitempty" db:"location"`
	Contact         *MessageContact      `json:"contact,omitempty" db:"contact"`
	Interactive     *MessageInteractive  `json:"interactive,omitempty" db:"interactive"`
	ReactedMetadata *InteractiveCallback `json:"reacted_metadata"`
	System          *MessageSystem       `json:"system,omitempty" db:"system"`
	Member          *ThreadDialog        `json:"member,omitempty" db:"member"`

	// DeliveryStatus is the aggregate across recipients: FAILED when every
	// recipient failed, otherwise the minimal status among non-failed ones.
	// Nil for messages without per-recipient tracking (historical).
	DeliveryStatus *MessageDeliveryStatus    `json:"delivery_status,omitempty" db:"delivery_status"`
	Statuses       []*MessageRecipientStatus `json:"statuses,omitempty" db:"statuses"`

	// Reactions holds the emoji reactions currently on the message, one per
	// reactor, ordered by first-reaction time.
	Reactions []*MessageReaction `json:"reactions,omitempty" db:"reactions"`

	// BotControllerMemberID is the thread_dialog.id of the currently active bot.
	// Included in MessageCreated events so flow_manager can self-filter.
	BotControllerMemberID *uuid.UUID `json:"-" db:"-"`

	ReplyTo *ReplyToPreview `json:"reply_to,omitempty" db:"reply_to"`

	ForwardOrigin *ForwardOrigin `json:"forward_origin,omitempty" db:"forward_origin"`

	domainEvents []event.Outboxer
}

type MessageSkipReason int16

const (
	MessageSkipUnspecified    MessageSkipReason = iota // 0: message was deleted, nothing skipped
	MessageSkipNotFound                                // 1: REASON_NOT_FOUND
	MessageSkipNotAuthor                               // 2: REASON_NOT_AUTHOR
	MessageSkipAlreadyDeleted                          // 3: REASON_ALREADY_DELETED
	MessageSkipChatClosed                              // 4: REASON_CHAT_CLOSED
	MessageSkipNotAllowed                              // 5: REASON_NOT_ALLOWED
)

var messageSkipReasonNames = map[MessageSkipReason]string{
	MessageSkipNotFound:       "not_found",
	MessageSkipNotAuthor:      "not_author",
	MessageSkipAlreadyDeleted: "already_deleted",
	MessageSkipChatClosed:     "chat_closed",
	MessageSkipNotAllowed:     "not_allowed",
}

func (r MessageSkipReason) String() string {
	if s, ok := messageSkipReasonNames[r]; ok {
		return s
	}

	return "unspecified"
}

type MessageSkip struct {
	ID     uuid.UUID
	Reason MessageSkipReason
}

type MessageDeleteResult struct {
	Deleted []*Message
	Skipped []MessageSkip
}

type ForwardOriginKind int16

const (
	ForwardOriginUnspecified ForwardOriginKind = iota
	ForwardOriginInternal
	ForwardOriginExternalUser
	ForwardOriginExternalHiddenUser
	ForwardOriginExternalChat
)

func (k ForwardOriginKind) IsExternal() bool {
	return k == ForwardOriginExternalUser ||
		k == ForwardOriginExternalHiddenUser ||
		k == ForwardOriginExternalChat
}

type ForwardOrigin struct {
	Kind            ForwardOriginKind `json:"kind"`
	SenderID        *uuid.UUID        `json:"sender_id,omitempty"`
	SenderName      string            `json:"sender_name,omitempty"`
	SenderIss       string            `json:"sender_iss,omitempty"`
	SenderSub       string            `json:"sender_sub,omitempty"`
	OriginalSentAt  int64             `json:"original_sent_at,omitempty"`
	SourceMessageID *uuid.UUID        `json:"source_message_id,omitempty"`
}

func NewInternalForwardOrigin(src *Message, senderName string) *ForwardOrigin {
	if src == nil {
		return nil
	}

	senderID, sourceID := src.SenderID, src.ID

	return &ForwardOrigin{
		Kind:            ForwardOriginInternal,
		SenderID:        &senderID,
		SenderName:      senderName,
		OriginalSentAt:  src.CreatedAtUnixMillis(),
		SourceMessageID: &sourceID,
	}
}

func (f *ForwardOrigin) AsEvent() *event.ForwardOriginPayload {
	if f == nil {
		return nil
	}

	return &event.ForwardOriginPayload{
		Kind:            int16(f.Kind),
		SenderID:        f.SenderID,
		SenderName:      f.SenderName,
		OriginalSentAt:  f.OriginalSentAt,
		SourceMessageID: f.SourceMessageID,
	}
}

type ReplyAttachment struct {
	Kind    string  `json:"kind"`
	Name    *string `json:"name,omitempty"`
	Mime    *string `json:"mime,omitempty"`
	Address *string `json:"address,omitempty"`
}

type ReplyToPreview struct {
	MessageID  uuid.UUID        `json:"message_id"`
	ThreadID   uuid.UUID        `json:"-" db:"thread_id"`
	SenderID   uuid.UUID        `json:"sender_id"`
	Type       MessageType      `json:"type"`
	Body       string           `json:"body"`
	CreatedAt  int64            `json:"created_at"`
	Attachment *ReplyAttachment `json:"attachment,omitempty"`
	IsDeleted  bool             `json:"is_deleted"`
}

func NewReplyTarget(id *uuid.UUID) *ReplyToPreview {
	if id == nil {
		return nil
	}

	return &ReplyToPreview{MessageID: *id}
}

func (m *Message) FirstViaOrDefault() string {
	for _, member := range m.To {
		if via := member.Via; via != nil && *via != "" {
			return *via
		}
	}

	return ""
}

func (m *Message) GetSender() uuid.UUID {
	if m.SendAs != nil && *m.SendAs != uuid.Nil {
		return *m.SendAs
	}

	return m.From.ID
}

func (m *Message) GetOriginSender() *uuid.UUID {
	if m.SendAs != nil && *m.SendAs != uuid.Nil {
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

func (m *Message) WithCreatedEvent(ctx context.Context, sendID string) *Message {
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
			IsBot:     member.IsBot,
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
				IsBot:     member.IsBot,
			}
		}
	}

	e := event.MessageCreated{
		MessageID:             m.ID,
		ThreadID:              m.ThreadID,
		DomainID:              m.DomainID,
		From:                  messageFrom,
		To:                    to,
		SendID:                sendID,
		Body:                  m.Body,
		Type:                  m.Type.String(),
		OccurredAt:            m.CreatedAt,
		Metadata:              maps.Clone(m.Metadata),
		BotControllerMemberID: m.BotControllerMemberID,
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

	if m.ReplyTo != nil {
		e.ReplyTo = &event.ReplyToPayload{
			MessageID: m.ReplyTo.MessageID,
			SenderID:  m.ReplyTo.SenderID,
			Type:      m.ReplyTo.Type.String(),
			Body:      m.ReplyTo.Body,
			CreatedAt: m.ReplyTo.CreatedAt,
			IsDeleted: m.ReplyTo.IsDeleted,
		}
		if a := m.ReplyTo.Attachment; a != nil {
			e.ReplyTo.AttachmentKind = &a.Kind
			e.ReplyTo.AttachmentName = a.Name
			e.ReplyTo.AttachmentMime = a.Mime
			e.ReplyTo.AttachmentAddress = a.Address
		}
	}

	e.ForwardOrigin = m.ForwardOrigin.AsEvent()

	if payload, ok := TryGetPayloadFromContext(ctx); ok {
		e.AddMetadata(XJWTPayload, payload)
	}

	WithContextPropogatedMetadata(ctx, &e)

	if via := m.FirstViaOrDefault(); via != "" && uuid.Validate(via) == nil {
		e.AddMetadata(XWebitelVia, via)
	}

	m.AddEvent(&e)

	return m
}

func (m *Message) WithEditedEvent(ctx context.Context) *Message {
	if m == nil {
		return m
	}

	e := event.MessageEdited{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		EditedBy:   m.actorAsEventMember(),
		To:         threadDialogsAsEventMembers(m.To),
		Body:       m.Body,
		Type:       m.Type.String(),
		Revision:   m.Version,
		CreatedAt:  m.CreatedAt,
		OccurredAt: m.UpdatedAt,
		Metadata:   maps.Clone(m.Metadata),
	}

	WithContextPropogatedMetadata(ctx, &e)

	if via := m.FirstViaOrDefault(); via != "" && uuid.Validate(via) == nil {
		e.AddMetadata(XWebitelVia, via)
	}

	m.AddEvent(&e)

	return m
}

func (m *Message) WithDeletedEvent(ctx context.Context, deleter *ContactIdentity) *Message {
	if m == nil {
		return m
	}

	e := event.MessageDeleted{
		MessageID:  m.ID,
		ThreadID:   m.ThreadID,
		DomainID:   m.DomainID,
		DeletedBy:  m.actorAsMember(deleter),
		To:         threadDialogsAsEventMembers(m.To),
		Type:       m.Type.String(),
		CreatedAt:  m.CreatedAt,
		OccurredAt: m.DeletedAtOrNow(),
	}

	WithContextPropogatedMetadata(ctx, &e)

	if via := m.FirstViaOrDefault(); via != "" && uuid.Validate(via) == nil {
		e.AddMetadata(XWebitelVia, via)
	}

	m.AddEvent(&e)

	return m
}

func (m *Message) DeletedAtOrNow() time.Time {
	if m.DeletedAt != nil {
		return *m.DeletedAt
	}

	return time.Now().UTC()
}

func (m *Message) DeletedAtUnixMillis() int64 {
	if m.DeletedAt == nil {
		return 0
	}

	return max(m.DeletedAt.UTC().UnixMilli(), 0)
}

func (m *Message) IsDeleted() bool {
	return m != nil && m.DeletedAt != nil
}

func (m *Message) actorAsEventMember() *event.ThreadMember {
	if m == nil || m.From.ID == uuid.Nil {
		return nil
	}

	actor := &event.ThreadMember{ContactID: m.From.ID}

	if m.MemberID != uuid.Nil {
		memberID := m.MemberID
		actor.ID = &memberID
	}

	for _, member := range m.To {
		if member == nil || member.ContactID != m.From.ID {
			continue
		}

		if member.ID != uuid.Nil {
			id := member.ID
			actor.ID = &id
		}

		actor.Role = int(member.ThreadRole)
		actor.IsBot = member.IsBot

		break
	}

	return actor
}

func (m *Message) actorAsMember(contact *ContactIdentity) *event.Member {
	if m == nil || m.From.ID == uuid.Nil {
		return nil
	}

	out := &event.Member{Role: UnspecifiedRole.String()}
	if m.MemberID != uuid.Nil {
		out.ID = m.MemberID.String()
	}

	isBot := false

	for _, member := range m.To {
		if member == nil || member.ContactID != m.From.ID {
			continue
		}

		if member.ID != uuid.Nil {
			out.ID = member.ID.String()
		}

		out.Role = member.ThreadRole.String()
		isBot = member.IsBot

		break
	}

	out.Contact = &event.MemberContact{ID: m.From.ID.String(), IsBot: isBot}

	if contact != nil {
		out.Contact.Sub = contact.Sub
		out.Contact.Iss = contact.Issuer
		out.Contact.Type = contact.Type
		out.Contact.Name = contact.Name
		out.Contact.Username = contact.Username
		out.Contact.IsBot = contact.IsBot
	}

	if identity := m.From.Identity; identity != nil {
		if out.Contact.Name == "" {
			out.Contact.Name = identity.Name
		}

		if out.Contact.Iss == "" {
			out.Contact.Iss = identity.Issuer
		}
	}

	return out
}

func threadDialogsAsEventMembers(members []*ThreadDialog) []*event.ThreadMember {
	out := make([]*event.ThreadMember, 0, len(members))
	for _, member := range members {
		var memberID *uuid.UUID

		if member.ID != uuid.Nil {
			id := member.ID
			memberID = &id
		}

		out = append(out, &event.ThreadMember{
			ID:        memberID,
			ContactID: member.ContactID,
			Role:      int(member.ThreadRole),
			IsBot:     member.IsBot,
		})
	}

	return out
}

func (m *MessageInteractive) AsEvent() *event.InteractivePayload {
	if m == nil {
		return nil
	}

	payload := &event.InteractivePayload{SingleUse: m.SingleUse}

	if m.Kind.Markup != nil {
		payload.Markup = mapMarkupToEvent(m.Kind.Markup)
	}

	if m.Kind.ListReply != nil {
		payload.ListReply = mapListReplyToEvent(m.Kind.ListReply)
	}

	return payload
}

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
