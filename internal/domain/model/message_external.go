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

type MessageDelivery struct {
	GateID     string
	ExternalID string
	Status     MessageDeliveryStatus
	Reason     string
	At         time.Time
	DomainID   int32
}
