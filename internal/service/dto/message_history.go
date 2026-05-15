package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	HistoryDocument struct {
		ID        uuid.UUID `json:"id" db:"id"`
		MessageID uuid.UUID `json:"message_id" db:"message_id"`
		FileID    int       `json:"file_id" db:"file_id"`
		Name      string    `json:"name" db:"name"`
		Mime      string    `json:"mime" db:"mime"`
		Size      int64     `json:"size" db:"size"`
		CreatedAt time.Time `json:"created_at" db:"created_at"`
		URL       string
	}

	HistoryImage struct {
		ID        uuid.UUID `json:"id" db:"id"`
		MessageID uuid.UUID `json:"message_id" db:"message_id"`
		FileID    int       `json:"file_id" db:"file_id"`
		Mime      string    `json:"mime" db:"mime"`
		Width     int       `json:"width" db:"width"`
		Height    int       `json:"height" db:"height"`
		CreatedAt time.Time `json:"created_at" db:"created_at"`
	}

	HistoryMessageMember struct {
		ID   uuid.UUID `json:"id" db:"id"`
		Role int       `json:"role" db:"role"`
	}

	HistoryMessage struct {
		ID        uuid.UUID      `json:"id" db:"id"`
		DomainID  int            `json:"domain_id" db:"domain_id"`
		ThreadID  uuid.UUID      `json:"thread_id" db:"thread_id"`
		SenderID  uuid.UUID      `json:"sender_id" db:"sender_id"`
		Type      int            `json:"type" db:"type"`
		Body      string         `json:"body" db:"body"`
		Metadata  map[string]any `json:"metadata" db:"metadata"`
		CreatedAt time.Time      `json:"created_at" db:"created_at"`
		UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`

		Documents []*HistoryDocument    `json:"documents" db:"documents"`
		Images    []*HistoryImage       `json:"images" db:"images"`
		Member    *HistoryMessageMember `json:"member,omitempty" db:"member"`
	}

	HistoryMessageInputDTO struct {
		DomainID  int
		Fields    []string
		Ids       uuid.UUIDs
		ThreadIds uuid.UUIDs
		SenderIds uuid.UUIDs
		Types     []int
		Cursor    *HistoryMessageCursor
		Sort      string
		Size      int
	}

	HistoryMessageCursor struct {
		CreatedAt time.Time
		Id        uuid.UUID
		Direction bool
	}

	DialogsMessageHistoryInputDTO struct {
		DomainID   int
		Fields     []string
		ThreadID   uuid.UUID
		SenderIds  uuid.UUIDs
		Types      []int
		PeriodFrom time.Time
		PeriodTo   time.Time
		Cursor     *HistoryMessageCursor
		Size       int
	}

	SessionMessageHistory struct {
		MemberID    uuid.UUID
		InvitedBy   uuid.UUID
		ThreadRole  model.ThreadRole
		LeaveReason string
		Messages    model.MessageSlice
	}

	DialogsMessageHistoryOutputDTO struct {
		Items      []*SessionMessageHistory
		From       []*model.ThreadDialog
		NextCursor string
		PrevCursor string
	}
)
