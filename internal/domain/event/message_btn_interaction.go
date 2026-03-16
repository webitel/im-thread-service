package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	MessageButtonInteractionCreated string = "im.msg_btn_interaction.created"
)

type MessageButtonInteraction struct {
	ID         uuid.UUID         `json:"id"`
	InReplyTo  uuid.UUID         `json:"in_reply_to"`
	DomainID   int               `json:"domain_id"`
	Action     string  			`json:"action"`
	ButtonCode string            `json:"button_code"`
	PressedBy  uuid.UUID         `json:"pressed_by"`
	PressedAt  time.Time         `json:"pressed_at"`
	Result     any `json:"result"`
}

func (mbi *MessageButtonInteraction) EventType() string { return  MessageButtonInteractionCreated }

func (mbi *MessageButtonInteraction) Version() string { return "v1" }

func (mbi *MessageButtonInteraction) RecipientID() uuid.UUID { return mbi.PressedBy }

func (mbi *MessageButtonInteraction) Topic() string {
	return fmt.Sprintf(
		"im_message.button.%s.%s.pressed.%s",
		mbi.Version(),
		mbi.Action,
		mbi.RecipientID().String(),
	)
}

func (mbi *MessageButtonInteraction) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(mbi)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:       uuid.Must(uuid.NewV7()),
		Payload:  payload,
		Metadata: map[string]string{
			"event_type": MessageButtonInteractionCreated,
			"version":    "v1",
		},
	}, nil
}