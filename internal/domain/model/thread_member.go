package model

import "github.com/google/uuid"

type ThreadMember struct {
	Id uuid.UUID

	DirectSettings *DirectThreadSetting
}
