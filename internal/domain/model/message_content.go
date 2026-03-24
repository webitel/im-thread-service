package model

import (
	"cmp"
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type Location struct {
	MessageID uuid.UUID `json:"message_id" db:"message_id"`
	Address   *string   `json:"address" db:"address"`
	Latitude  float64   `json:"latitude" db:"latitude"`
	Longitude float64   `json:"longitude" db:"longitude"`
	Name      *string   `json:"name" db:"name"`
}

type Contact struct {
	MessageID   uuid.UUID         `json:"message_id" db:"message_id"`
	Name        *string           `json:"name" db:"name"`
	Email       *string           `json:"email" db:"email"`
	PhoneNumber *string           `json:"phone_number" db:"phone_number"`
	Metadata    map[string]string `json:"metadata" db:"metadata"`
}

func (c *Contact) Validate() error {
	if c == nil {
		return errors.InvalidArgument("contact is null")
	}

	v := cmp.Or(
		c.Name, c.PhoneNumber, c.Email,
	)

	if v == nil {
		return errors.InvalidArgument("one of: email, phone_number or name must be filled")
	}

	return nil
}

type Interactive struct {
	Header    *InteractiveHeader `json:"header"`
	Body      string             `json:"-"`
	Footer    *string            `json:"footer"`
	SingleUse bool               `json:"single_use"`
	CTA       *KeyboardButton    `json:"cta,omitempty"`
	ListReply *KeyboardListReply `json:"list_reply,omitempty"`
	Markup    *KeyboardMarkup    `json:"markup,omitempty"`
}

func (i *Interactive) Value() (driver.Value, error) {
	var (
		raw []byte
		err error
	)

	if raw, err = json.Marshal(i); err != nil {
		return nil, errors.Internal("error marshaling interactive payload")
	}

	return raw, nil
}

const (
	InteractiveTypeMarkup    string = "markup"
	InteractiveTypeListReply string = "list_reply"
	InteractiveTypeCTA       string = "cta"
)

type InteractiveTopologist interface {
	Type() string
}

type InteractiveHeader struct {
	Documents []*MessageDocument `json:"documents,omitempty"`
	Images    []*MessageImage    `json:"images,omitempty"`
	Text      *string            `json:"text,omitempty"`
}

type KeyboardMask interface {
	KeyboardButton | KeyboardListReply | KeyboardMarkup
}

type KeyboardMarkup struct {
	ButtonsMatrix [][]*KeyboardButton `json:"buttons_matrix"`
}

func (k *KeyboardMarkup) Type() string { return InteractiveTypeMarkup }

type KeyboardListReply struct {
	MainButtonTitle string                     `json:"main_button_title"`
	Sections        []*KeyboardRowWithSections `json:"sections"`
}

func (k *KeyboardListReply) Type() string { return InteractiveTypeListReply }

type KeyboardRowWithSections struct {
	Section string            `json:"section"`
	Buttons []*KeyboardButton `json:"buttons"`
}

type KeyboardButton struct {
	ButtonID        string                   `json:"button_id"`
	ButtonLabelText string                   `json:"button_label_text"`
	Kind            KeyboardButtonIdentifier `json:"-"`
	URL             *KeyboardButtonURL       `json:"url,omitempty"`
	Callback        *KeyboardButtonCallback  `json:"callback,omitempty"`
	Request         *KeyboardButtonRequest   `json:"request,omitempty"`
	Metadata        map[string]any           `json:"metadata,omitempty"`
}

func (k *KeyboardButton) Type() string { return InteractiveTypeCTA }

const (
	ButtonTypeCallback string = "callback"
	ButtonTypeURL      string = "url"
	ButtonTypeRequest  string = "request"
)

type KeyboardButtonIdentifier interface {
	Type() string
}

type KeyboardButtonURL struct {
	URL string `json:"url"`
}

func (k *KeyboardButtonURL) Type() string { return ButtonTypeURL }

type KeyboardButtonCallback struct {
	Data string `json:"data"`
}

func (k *KeyboardButtonCallback) Type() string { return ButtonTypeCallback }

type KeyboardButtonRequestAction string

const (
	UnknownButtonRequestAction  KeyboardButtonRequestAction = "unknown"
	ContactButtonRequestAction  KeyboardButtonRequestAction = "contact"
	LocationButtonRequestAction KeyboardButtonRequestAction = "location"
)

type KeyboardButtonRequest struct {
	Action KeyboardButtonRequestAction `json:"action"`
}

func (k *KeyboardButtonRequest) Type() string { return ButtonTypeRequest }

type ButtonsCallback struct {
	MessageID    uuid.UUID `json:"message_id" db:"message_id"`
	ButtonCode   string    `json:"button_code" db:"button_code"`
	CallbackData string    `json:"callback_data" db:"callback_data"`
	ClickedAt    time.Time `json:"clicked_at" db:"clicked_at" fieldtag:"ign"`
	ClickedBy    uuid.UUID `json:"clicked_by" db:"clicked_by"`
}

func (bc *ButtonsCallback) GetClickedAtUnix() int64 { return bc.ClickedAt.UTC().UnixMilli() }