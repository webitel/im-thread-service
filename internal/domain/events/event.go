package events

import (
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type Event interface {
	EventType() string
}

const MessageCreatedEvent = "im.message.created"

type MessageCreated struct {
	MessageID  uuid.UUID         `json:"message_id"`
	ThreadID   uuid.UUID         `json:"thread_id"`
	From       model.Peer        `json:"from"`
	To         model.Peer        `json:"to"`
	Body       string            `json:"body"`
	Type       model.MessageType `json:"type"`
	OccurredAt time.Time         `json:"occurred_at"`
}

func (MessageCreated) EventType() string {
	return MessageCreatedEvent
}
