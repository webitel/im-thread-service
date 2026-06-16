package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const BotControlReleasedEvent = "im.thread.bot.control.released"

type BotControlReleased struct {
	ThreadID     uuid.UUID  `json:"thread_id"`
	DomainID     int32      `json:"domain_id"`
	MemberID     uuid.UUID  `json:"member_id"`
	Position     int        `json:"position"`
	Reason       string     `json:"reason"`
	NextMemberID *uuid.UUID `json:"next_member_id,omitempty"`
	OccurredAt   time.Time  `json:"occurred_at"`
}

var _ Base = (*BotControlReleased)(nil)

func (e *BotControlReleased) EventType() string { return BotControlReleasedEvent }

func (e *BotControlReleased) Version() string { return MessageVersionV1 }

func (e *BotControlReleased) RecipientID() uuid.UUID { return e.ThreadID }

func (e *BotControlReleased) MustBeThreadEvent() {}

func (e *BotControlReleased) Topic() string {
	return fmt.Sprintf("im_thread.%s.bot.control.released.%s", e.ThreadID, e.Version())
}

func (e *BotControlReleased) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": BotControlReleasedEvent,
			"version":    e.Version(),
		},
	}, nil
}
