package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

var _ Outboxer = (*InteractiveCallbackPayload)(nil)

type InteractiveCallbackPayload struct {
	ReactedBy    shared.Peer `json:"reacted_by"`
	InReplyTo    uuid.UUID   `json:"in_reply_to"`
	ButtonCode   string      `json:"button_code"`
	CallbackData string      `json:"callback_data"`
	ReactedAt    time.Time   `json:"reacted_at"`
	DomainID     int         `json:"domain_id"`
	ThreadID     uuid.UUID   `json:"thread_id"`
	Receiver     uuid.UUID   `json:"receiver"`
}

func (i *InteractiveCallbackPayload) EventType() string {
	return "reacted"
}

func (i *InteractiveCallbackPayload) RecipientID() uuid.UUID {
	return i.InReplyTo
}

func (i *InteractiveCallbackPayload) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(i)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": i.EventType(),
			"version":    i.Version(),
		},
	}, nil
}

func (i *InteractiveCallbackPayload) Version() string {
	return "v1"
}
