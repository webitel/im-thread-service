package model

import "github.com/google/uuid"

type Thread struct {
	BaseModel

	Owner   uuid.UUID   `json:"owner"`
	Admins  []uuid.UUID `json:"admins"`
	Members []uuid.UUID `json:"members"`

	Subject     uuid.UUID `json:"subject"`
	Description string    `json:"description"`
}
