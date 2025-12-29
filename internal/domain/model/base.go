package model

import (
	"time"

	"github.com/google/uuid"
)

type BaseModel struct {
	Id        uuid.UUID `json:"id"`
	CreatedBy uuid.UUID `json:"created_by"`
	UpdatedBy uuid.UUID `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
