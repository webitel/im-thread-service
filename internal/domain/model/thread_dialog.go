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

	DirectThreadDialog struct {
		ThreadDialog

		MemberSettings   *DirectThreadSetting
		DirectToSettings *DirectThreadSetting
	}

	DirectThreadDialogBuilder struct {
		thread *DirectThreadDialog
	}
)

func NewDirectThreadDialogBuilder() *DirectThreadDialogBuilder {
	return &DirectThreadDialogBuilder{
		thread: new(DirectThreadDialog),
	}
}

func (b *DirectThreadDialogBuilder) WithDomainID(domainID int) *DirectThreadDialogBuilder {
	if domainID > 0 {
		b.thread.DomainID = domainID
	}

	return b
}

func (b *DirectThreadDialogBuilder) WithThreadDialog(threadDialog *ThreadDialog) *DirectThreadDialogBuilder {
	if threadDialog == nil {
		return b
	}

	b.thread.ThreadDialog = *threadDialog

	return b
}

func (b *DirectThreadDialogBuilder) WithMemberSettings(settings *DirectThreadSetting) *DirectThreadDialogBuilder {
	if settings == nil {
		return b
	}

	settingsVal := *settings

	b.thread.MemberSettings = &settingsVal

	return b
}

func (b *DirectThreadDialogBuilder) WithDirectToSettings(settings *DirectThreadSetting) *DirectThreadDialogBuilder {
	if settings == nil {
		return b
	}

	settingsVal := *settings

	b.thread.DirectToSettings = &settingsVal

	return b
}

func (b *DirectThreadDialogBuilder) Build() *DirectThreadDialog {
	return b.thread
}
