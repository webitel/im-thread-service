package model

import (
	"fmt"
	"slices"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/webitel-go-kit/pkg/errors"
	"google.golang.org/grpc/codes"
)

const (
	MaxButtonCodeLen         int = 255
	MAxCallbackDataLen       int = 1000
	MaxLocationInfoLen       int = 100
	MaxLocationPostalCodeLen int = 100
)



var (
	ErrInteractionConflict = errors.New(
		"interaction for this message already exists",
		errors.WithCode(codes.AlreadyExists),
	)
)

// #region MessageButtonInteraction 

type MessageButtonInteraction struct {
	ID         uuid.UUID         `json:"id" db:"id" fieldtag:"ign"`
	InReplyTo  uuid.UUID         `json:"in_reply_to" db:"in_reply_to"`
	DomainID   int               `json:"domain_id" db:"domain_id"`
	Action     ButtonActionType  `json:"action" db:"action"`
	ButtonCode string            `json:"button_code" db:"button_code"`
	PressedBy  uuid.UUID         `json:"pressed_by" db:"pressed_by"`
	PressedAt  time.Time         `json:"pressed_at" db:"pressed_at" fieldtag:"ign"`
	Result     InteractionResult `json:"result" db:"-"`

	domainEvents []event.Base
	lock sync.RWMutex
}

func (mbi *MessageButtonInteraction) Events() []event.Base { 
	mbi.lock.RLock()
	defer mbi.lock.RUnlock()
	
	return slices.Clone(mbi.domainEvents) 
}

func (mbi *MessageButtonInteraction) AddCreatedEvent() *MessageButtonInteraction {
	e := &event.MessageButtonInteraction{
		ID:         mbi.ID,
		InReplyTo:  mbi.InReplyTo,
		DomainID:   mbi.DomainID,
		Action:     string(mbi.Action),
		ButtonCode: mbi.ButtonCode,
		PressedBy:  mbi.PressedBy,
		PressedAt:  mbi.PressedAt,
		Result:     mbi.Result,
	}

	
	mbi.lock.Lock()
	mbi.domainEvents = append(mbi.domainEvents, e)
	mbi.lock.Unlock()
	
	return mbi
}

func (mbi *MessageButtonInteraction) Validate() error {
	if mbi == nil {
		return errors.InvalidArgument("message button interaction is null")
	}

	if mbi.InReplyTo == uuid.Nil {
		return errors.InvalidArgument("in_reply_to_message is required")
	}

	if mbi.DomainID <= 0 {
		return errors.InvalidArgument("domain_id must be gt then 0")
	}

	if mbi.ButtonCode == "" || !utf8.ValidString(mbi.ButtonCode) {
		return errors.InvalidArgument("invalid button_code format")
	}

	if utf8.RuneCountInString(mbi.ButtonCode) > MaxButtonCodeLen {
		return errors.InvalidArgument(
			fmt.Sprintf("button_code len can`t be gt then %d", MaxButtonCodeLen),
		)
	}

	if mbi.PressedBy == uuid.Nil {
		return errors.InvalidArgument("pressed_by is required")
	}

	if err := mbi.Result.Validate(); err != nil {
		return err
	}

	return nil
}

func (mbi *MessageButtonInteraction) GetPressedAt() int64 { return mbi.PressedAt.UTC().UnixMilli() }


// #endregion

type InteractionResult interface {
	Validator
	Type() ButtonActionType
}

type InteractionPostback struct {
	InteractionID uuid.UUID `json:"interaction_id" db:"interaction_id"`
	CallbackData  string    `json:"callback_data" db:"callback_data"`
}

func (ip *InteractionPostback) Type() ButtonActionType { return PostbackAction }

func (ip *InteractionPostback) Validate() error {
	if ip == nil {
		return errors.InvalidArgument("postback action is null")
	}

	if ip.InteractionID == uuid.Nil {
		return errors.InvalidArgument("interaction id is in incorrect format")
	}

	if ip.CallbackData == "" || !utf8.ValidString(ip.CallbackData) {
		return errors.InvalidArgument("callback data has invalid format")
	}

	if utf8.RuneCountInString(ip.CallbackData) > MAxCallbackDataLen {
		return errors.InvalidArgument("callback data is to large")
	}

	return nil
}

type InteractionContact struct {
	InteractionID uuid.UUID         `json:"interaction_id" db:"interaction_id"`
	Name          string            `json:"name" db:"name"`
	PhoneNumber   string            `json:"phone_number" db:"phone_number"`
	Metadata      map[string]string `json:"metadata" db:"metadata"`
}

func (ic *InteractionContact) Type() ButtonActionType { return ContactAction }

func (ic *InteractionContact) Validate() error {
	if ic == nil {
		return errors.InvalidArgument("contact interaction is null")
	}

	if ic.Name != "" && !utf8.ValidString(ic.Name) {
		return errors.InvalidArgument("contact name has invalid utf-8 sequence")
	}

	return nil
}

type InteractionLocation struct {
	InteractionID uuid.UUID `json:"interaction_id" db:"interaction_id"`
	Latitude      float64   `json:"latitude" db:"latitude"`
	Longitude     float64   `json:"longitude" db:"longitude"`
	City          *string   `json:"city" db:"city"`
	State         *string   `json:"state" db:"state"`
	Country       *string   `json:"country" db:"country"`
	PostalCode    *string   `json:"postal_code" db:"postal_code"`
}

func (il *InteractionLocation) Type() ButtonActionType { return LocationAction }

func (il *InteractionLocation) Validate() error {
	if il == nil {
		return errors.InvalidArgument("location interaction is null")
	}

	for key, value := range map[string]*string{
		"city":    il.City,
		"state":   il.State,
		"country": il.Country,
	} {
		if err := il.checkGenericStrConstraints(key, value); err != nil {
			return err
		}
	}

	if il.PostalCode != nil &&
		(*il.PostalCode == "" || utf8.RuneCountInString(*il.PostalCode) > MaxLocationPostalCodeLen || !utf8.ValidString(*il.PostalCode)) {
		return errors.InvalidArgument("postal code attribute has invalid form or incorrect utf-8 sequence")
	}

	if il.Latitude < -90 || il.Latitude > 90 {
		return errors.InvalidArgument("latitude must be between -90 and 90")
	}

	if il.Longitude < -180 || il.Longitude > 180 {
		return errors.InvalidArgument("longitude must be between -180 and 180")
	}

	return nil
}

func (il *InteractionLocation) checkGenericStrConstraints(attr string, value *string) error {
	if value == nil {
		return nil
	}

	if *value == "" {
		return errors.InvalidArgument(
			fmt.Sprintf("location attribute %s can`t be empty string", attr),
		)
	}

	if utf8.RuneCountInString(*value) > MaxLocationInfoLen {
		return errors.InvalidArgument(
			fmt.Sprintf("location attribute %s has longer characters sequence than allowed %d", attr, MaxLocationInfoLen),
		)
	}

	if !utf8.ValidString(*value) {
		return errors.InvalidArgument(
			fmt.Sprintf("location attribute %s has invalid utf-8 sequence", attr),
		)
	}

	return nil
}
