package model

import (
	"time"

	"github.com/google/uuid"
)

type (
	DirectThreadSetting struct {
		BaseThreadSetting
		Title string `json:"title" db:"title"`
		ID uuid.UUID `json:"id" db:"id"`
		UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	}

	directThreadSettingBuilder struct {
		setting *DirectThreadSetting
	}
)

func NewDirectThreadSettingBuilder() *directThreadSettingBuilder {
	return &directThreadSettingBuilder{
		setting: new(DirectThreadSetting),
	}
}

func (b *directThreadSettingBuilder) WithBaseSettings(settings *BaseThreadSetting) *directThreadSettingBuilder {
	if settings != nil {
		b.setting.BaseThreadSetting = *settings
	}

	return b
}

func (b *directThreadSettingBuilder) Build() *DirectThreadSetting {
	return b.setting
}
