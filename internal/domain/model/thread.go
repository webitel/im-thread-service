package model

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type ThreadKind = int

const (
	ThreadDirect  ThreadKind = iota // user to user
	ThreadGroup                     // group with many users, bots etc.
	ThreadChannel                   // channel, right now not implemented!
)

type Thread struct {
	shared.BaseModel

		Kind    ThreadKind  `json:"kind"`
		Owner   uuid.UUID   `json:"owner"`
		Admins  []uuid.UUID `json:"admins"`
		Members []uuid.UUID `json:"members"`

	Subject     string `json:"subject"`
	Description string `json:"description"`
}
