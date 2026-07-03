package event

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/google/uuid"
)

const (
	MessageVersionV1 = "v1"
)

const (
	MessageCreatedEvent = "im.message.created"
)

type Outboxer interface {
	EventType() string
	Version() string
	RecipientID() uuid.UUID
	ToOutbox() (OutboxEvent, error)
}

type OutboxerTemplate interface {
	Outboxer
	SetRecipientID(uuid.UUID) Outboxer
}

type OutboxEvent struct {
	ID       uuid.UUID
	Payload  []byte
	Metadata map[string]string
}

var _ Outboxer = (*MessageCreated)(nil)

type MessageCreated struct {
	MessageID   uuid.UUID           `json:"message_id"`
	ThreadID    uuid.UUID           `json:"thread_id"`
	DomainID    int32               `json:"domain_id"`
	From        *ThreadMember       `json:"from"`
	To          []*ThreadMember     `json:"to"`
	SendID      string              `json:"send_id"`
	Body        string              `json:"body"`
	Type        string              `json:"type"` // text|document|image|system|interactive|location|contact
	OccurredAt  time.Time           `json:"occurred_at"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
	Images      []ImagePayload      `json:"images,omitempty"`
	Documents   []DocumentPayload   `json:"documents,omitempty"`
	Location    *LocationPayload    `json:"location,omitempty"`
	Contact     *ContactPayload     `json:"contact,omitempty"`
	Interactive *InteractivePayload `json:"interactive,omitempty"`
	System      *SystemPayload      `json:"system,omitempty"`
	// BotControllerMemberID is the members[].id (thread membership record ID) of the active bot controller.
	// Matches the member_id field in bot.control.granted.v1 events.
	// flow_manager compares this against its own member_id to decide whether to process the message.
	BotControllerMemberID *uuid.UUID        `json:"bot_controller_member_id,omitempty"`
	ExternalMetadata      map[string]string `json:"-"`
}

func (m *MessageCreated) AddMetadata(key, value string) {
	if m.ExternalMetadata == nil {
		m.ExternalMetadata = make(map[string]string)
	}

	m.ExternalMetadata[key] = value
}

type ThreadMember struct {
	ID        *uuid.UUID `json:"member_id,omitempty"`
	ContactID uuid.UUID  `json:"contact_id"`
	Role      int        `json:"role"`
	IsBot     bool       `json:"is_bot"`
}

func (*MessageCreated) EventType() string                { return MessageCreatedEvent }
func (m *MessageCreated) Version() string                { return MessageVersionV1 }
func (m *MessageCreated) RecipientID() uuid.UUID         { return m.ThreadID }
func (m *MessageCreated) ToOutbox() (OutboxEvent, error) { return m.serialize(m, m.Version()) }

func (m *MessageCreated) serialize(data any, version string) (OutboxEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return OutboxEvent{}, err
	}

	outboxMetadata := map[string]string{
		"event_type": MessageCreatedEvent,
		"version":    version,
	}

	maps.Copy(outboxMetadata, m.ExternalMetadata)

	return OutboxEvent{
		ID:       uuid.Must(uuid.NewV7()),
		Payload:  payload,
		Metadata: outboxMetadata,
	}, nil
}

type ImagePayload struct {
	FileID int64  `json:"file_id"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

type DocumentPayload struct {
	FileID int64  `json:"file_id"`
	Mime   string `json:"mime"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	URL    string `json:"url"`
}

type LocationPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   *string `json:"address,omitempty"`
	Name      *string `json:"name,omitempty"`
}

type ContactPayload struct {
	Name  *string `json:"name,omitempty"`
	Phone *string `json:"phone,omitempty"`
	Email *string `json:"email,omitempty"`
}

func NewContactPayload(name, phone, email *string) *ContactPayload {
	return &ContactPayload{
		Name:  name,
		Phone: phone,
		Email: email,
	}
}

type SystemPayload struct {
	Type     string         `json:"type"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func NewSystemPayload(systemType string, metadata map[string]any) *SystemPayload {
	return &SystemPayload{
		Type:     systemType,
		Metadata: metadata,
	}
}

func NewLocationPayload(latitude, longitude float64, address, name *string) *LocationPayload {
	return &LocationPayload{
		Latitude:  latitude,
		Longitude: longitude,
		Address:   address,
		Name:      name,
	}
}

const (
	ActionTypeURL      = "url"
	ActionTypeCallback = "callback"
	ActionTypeRequest  = "request"
)

type KeyboardButtonURL struct {
	URL string `json:"url"`
}

type KeyboardButtonCallback struct {
	Data string `json:"data"`
}

type KeyboardButtonRequest struct {
	Action string `json:"action"`
}

type KeyboardButton struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata,omitempty"`

	Type     string                  `json:"type"`
	URL      *KeyboardButtonURL      `json:"url,omitempty"`
	Callback *KeyboardButtonCallback `json:"callback,omitempty"`
	Request  *KeyboardButtonRequest  `json:"request,omitempty"`
}

type KeyboardRow struct {
	Buttons []*KeyboardButton `json:"buttons"`
}

type KeyboardRowWithSection struct {
	Section string            `json:"section"`
	Buttons []*KeyboardButton `json:"buttons"`
}

type KeyboardListReply struct {
	MainButtonTitle string                    `json:"main_button_title"`
	Sections        []*KeyboardRowWithSection `json:"sections"`
}

type KeyboardMarkup struct {
	Rows []*KeyboardRow `json:"rows"`
}

type InteractivePayload struct {
	SingleUse bool               `json:"single_use"`
	Markup    *KeyboardMarkup    `json:"markup,omitempty"`
	ListReply *KeyboardListReply `json:"list_reply,omitempty"`
}
