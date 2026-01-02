package model

import "github.com/google/uuid"

type ThreadDialog struct {
	BaseModel

	MemberId uuid.UUID `json:"member_id"`
	ThreadId uuid.UUID `json:"thread_id"`
	MemberOf *uuid.UUID `json:"member_of"` //NULLABLE IN CASE IF DIRECT THREAD
	DirectTo *uuid.UUID `json:"direct_to"` //NULLABLE IN CASE IF GROUP/CHANNEL THREAD
}

