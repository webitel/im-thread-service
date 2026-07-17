package model

import "github.com/google/uuid"

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
