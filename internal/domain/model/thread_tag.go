package model

import (
	"time"

	"github.com/google/uuid"
)

type ThreadTag struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ThreadID  uuid.UUID `json:"thread_id" db:"thread_id"`
	ContactID uuid.UUID `json:"contact_id" db:"contact_id"`
	Tag       string    `json:"tag" db:"tag"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

func (t *ThreadTag) CreatedAtUnix() int64 {
	return t.CreatedAt.UnixMilli()
}
