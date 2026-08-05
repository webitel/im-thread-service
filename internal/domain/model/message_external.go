package model

import (
	"time"

	"github.com/google/uuid"
)

type MessageExternalDirection int16

const (
	ExternalDirectionInbound  MessageExternalDirection = 1
	ExternalDirectionOutbound MessageExternalDirection = 2
)

type MessageExternalID struct {
	MessageID  uuid.UUID
	ThreadID   uuid.UUID
	GateID     string
	ExternalID string
	Direction  MessageExternalDirection
}

type MessageDeliveryStatus int16

const (
	MessageDeliveryUnspecified MessageDeliveryStatus = 0
	MessageDeliveryDelivered   MessageDeliveryStatus = 1
	MessageDeliveryRead        MessageDeliveryStatus = 2
	MessageDeliveryFailed      MessageDeliveryStatus = 3
)

func (s MessageDeliveryStatus) String() string {
	switch s {
	case MessageDeliveryDelivered:
		return "delivered"
	case MessageDeliveryRead:
		return "read"
	case MessageDeliveryFailed:
		return "failed"
	default:
		return "unspecified"
	}
}

func (s MessageDeliveryStatus) Valid() bool {
	switch s {
	case MessageDeliveryDelivered, MessageDeliveryRead, MessageDeliveryFailed:
		return true
	default:
		return false
	}
}

type MessageDelivery struct {
	GateID     string
	ExternalID string
	Status     MessageDeliveryStatus
	Reason     string
	At         time.Time
	DomainID   int32
}
