package model

import "github.com/google/uuid"

type ThreadMember struct {
	Id uuid.UUID `json:"id" db:"id"`

	DirectSettings *DirectThreadSetting `json:"direct_settings,omitempty" db:"direct_settings"`
}
