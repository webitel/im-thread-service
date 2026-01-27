package model

import (
	"time"

	"github.com/google/uuid"
)

type (
	BaseThreadSetting struct {
		BaseModel

		ThreadDialogID uuid.UUID
		Title          string
	}

	baseThreadSettingBuilder struct {
		setting *BaseThreadSetting
	}
)

func NewBaseThreadSettingBuilder() *baseThreadSettingBuilder {
	return &baseThreadSettingBuilder{
		setting: new(BaseThreadSetting),
	}
}

func (b *baseThreadSettingBuilder) WithID(id uuid.UUID) *baseThreadSettingBuilder {
	if id != uuid.Nil {
		b.setting.ID = id
	}

	return b
}

func (b *baseThreadSettingBuilder) WithDomainID(id int) *baseThreadSettingBuilder {
	if id > 0 {
		b.setting.DomainID = id
	}

	return b
}

func (b *baseThreadSettingBuilder) WithCreatedAt(createdAt time.Time) *baseThreadSettingBuilder {
	b.setting.CreatedAt = createdAt

	return b
}

func (b *baseThreadSettingBuilder) WithUpdatedAt(updatedAt time.Time) *baseThreadSettingBuilder {
	b.setting.UpdatedAt = updatedAt

	return b
}

func (b *baseThreadSettingBuilder) WithThreadDialogID(id uuid.UUID) *baseThreadSettingBuilder {
	if id != uuid.Nil {
		b.setting.ThreadDialogID = id
	}

	return b
}

func (b *baseThreadSettingBuilder) WithTitle(title string) *baseThreadSettingBuilder {
	b.setting.Title = title

	return b
}

func (b *baseThreadSettingBuilder) Build() *BaseThreadSetting {
	return b.setting
}
