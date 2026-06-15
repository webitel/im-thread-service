package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const BotControlGrantedEvent = "im.thread.bot.control.granted"

type BotControlGranted struct {
	ThreadID         uuid.UUID  `json:"thread_id"`
	DomainID         int32      `json:"domain_id"`
	MemberID         uuid.UUID  `json:"member_id"`
	ContactID        uuid.UUID  `json:"contact_id"`
	Position         int        `json:"position"`
	AutoLeave        bool       `json:"auto_leave"`
	Reason           string     `json:"reason"`
	IsResume         bool       `json:"is_resume"`
	PreviousPosition *int       `json:"previous_position,omitempty"`
	PreviousMemberID *uuid.UUID `json:"previous_member_id,omitempty"`
	// ControlEpoch is a monotonic per-thread counter incremented on every grant.
	// flow_manager must pass this value back in CompleteBotControl to prevent
	// stale or duplicate requests from being accepted (ABA protection).
	ControlEpoch     int64      `json:"control_epoch"`
	OccurredAt       time.Time  `json:"occurred_at"`
}

var _ Base = (*BotControlGranted)(nil)

func (e *BotControlGranted) EventType() string { return BotControlGrantedEvent }

func (e *BotControlGranted) Version() string { return MessageVersionV1 }

func (e *BotControlGranted) RecipientID() uuid.UUID { return e.ThreadID }

func (e *BotControlGranted) MustBeThreadEvent() {}

func (e *BotControlGranted) Topic() string {
	return fmt.Sprintf("im_thread.%s.bot.control.granted.%s", e.ThreadID, e.Version())
}

func (e *BotControlGranted) ToOutbox() (OutboxEvent, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:      uuid.Must(uuid.NewV7()),
		Payload: payload,
		Metadata: map[string]string{
			"event_type": BotControlGrantedEvent,
			"version":    e.Version(),
		},
	}, nil
}
