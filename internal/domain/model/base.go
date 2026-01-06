package model

import (
	"time"

	"github.com/google/uuid"
)

type BaseModel struct {
	Id        uuid.UUID `json:"id"`
	DomainId  int       `json:"domain_id"`
	CreatedBy int `json:"created_by"`
	UpdatedBy int `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
