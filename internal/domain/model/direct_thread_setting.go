package model

type (
	DirectThreadSetting struct {
		BaseThreadSetting
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
