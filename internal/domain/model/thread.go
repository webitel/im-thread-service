package model

import (
	"github.com/google/uuid"
)

type ThreadKind = int

const (
	ThreadUnspecified ThreadKind = iota
	ThreadDirect                 // user to user
	ThreadGroup                  //group with many users, bots etc.
	ThreadChannel                //channel, right now not implemented!
)

type (
	Thread struct {
		BaseModel

		Kind       ThreadKind      `json:"kind"`
		Owner      uuid.UUID       `json:"owner"`
		Admins     []uuid.UUID     `json:"admins"`
		MembersIds []uuid.UUID     `json:"members_ids"`
		Members    []*ThreadMember `json:"members"`

		Subject     string `json:"subject"`
		Description string `json:"description"`
	}
)
