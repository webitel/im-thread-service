package model

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type ThreadDialog struct {
	shared.BaseModel

	MemberID uuid.UUID  `json:"member_id"`
	ThreadID uuid.UUID  `json:"thread_id"`
	MemberOf *uuid.UUID `json:"member_of"` // NULLABLE IN CASE IF DIRECT THREAD
	DirectTo *uuid.UUID `json:"direct_to"` // NULLABLE IN CASE IF GROUP/CHANNEL THREAD
}
