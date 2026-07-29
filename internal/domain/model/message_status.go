package model

import (
	"time"

	"github.com/google/uuid"
)

// MessageDeliveryStatus is a per-recipient delivery state of a message.
// Transitions are monotonic: SENT -> DELIVERED -> READ.
// FAILED is terminal but may be superseded by a later DELIVERED (retry).
type MessageDeliveryStatus int16

const (
	MessageDeliveryStatusUnspecified MessageDeliveryStatus = iota
	MessageDeliveryStatusSent                              // 1: SENT
	MessageDeliveryStatusDelivered                         // 2: DELIVERED
	MessageDeliveryStatusRead                              // 3: READ
	MessageDeliveryStatusFailed                            // 4: FAILED
)

var messageDeliveryStatusNames = map[MessageDeliveryStatus]string{
	MessageDeliveryStatusSent:      "sent",
	MessageDeliveryStatusDelivered: "delivered",
	MessageDeliveryStatusRead:      "read",
	MessageDeliveryStatusFailed:    "failed",
}

func (s MessageDeliveryStatus) String() string {
	if name, ok := messageDeliveryStatusNames[s]; ok {
		return name
	}

	return "unspecified"
}

// MessageStatus is a single per-recipient delivery state row.
type MessageStatus struct {
	DomainID    int32                 `json:"domain_id" db:"domain_id"`
	ThreadID    uuid.UUID             `json:"thread_id" db:"thread_id"`
	MessageID   uuid.UUID             `json:"message_id" db:"message_id"`
	MemberID    uuid.UUID             `json:"member_id" db:"member_id"`
	Status      MessageDeliveryStatus `json:"status" db:"status"`
	DeliveredAt *time.Time            `json:"delivered_at,omitempty" db:"delivered_at"`
	ReadAt      *time.Time            `json:"read_at,omitempty" db:"read_at"`
	FailedAt    *time.Time            `json:"failed_at,omitempty" db:"failed_at"`
	Error       map[string]any        `json:"error,omitempty" db:"error"`
	Via         *string               `json:"via,omitempty" db:"via"`
	UpdatedAt   time.Time             `json:"updated_at" db:"updated_at"`
}

// MessageRecipientStatus is the per-recipient delivery detail attached to
// history messages, decoded from the jsonb aggregate of v_messages.
type MessageRecipientStatus struct {
	MemberID    uuid.UUID             `json:"member_id"`
	Status      MessageDeliveryStatus `json:"status"`
	DeliveredAt *time.Time            `json:"delivered_at,omitempty"`
	ReadAt      *time.Time            `json:"read_at,omitempty"`
	FailedAt    *time.Time            `json:"failed_at,omitempty"`
	Via         *string               `json:"via,omitempty"`
	Error       map[string]any        `json:"error,omitempty"`
}

// StatusReceipt is a delivery/failure confirmation for a single message
// and recipient, reported by im-delivery or im-providers.
type StatusReceipt struct {
	DomainID  int32
	ThreadID  uuid.UUID
	MessageID uuid.UUID
	// MemberID is the recipient contact id (thread_dialog.member_id).
	MemberID uuid.UUID
	// At is the confirmation time; zero means "now".
	At time.Time
	// Via is the confirmation source: ws|push|provider|bot.
	Via string
	// Error carries provider error details for failure receipts.
	Error map[string]any
}

// ReadReceipt confirms that a recipient has read the thread up to a message
// (inclusive): every earlier unread message of the recipient is covered.
type ReadReceipt struct {
	DomainID int32
	ThreadID uuid.UUID
	// MemberID is the recipient contact id (thread_dialog.member_id).
	MemberID      uuid.UUID
	UpToMessageID uuid.UUID
	// At is the confirmation time; zero means "now".
	At time.Time
	// Via is the confirmation source: ws|push|provider|bot.
	Via string
}

// StatusChange is a status row that was actually changed by an upsert.
// Duplicate and out-of-order receipts produce no changes.
type StatusChange struct {
	DomainID  int32                 `db:"domain_id"`
	ThreadID  uuid.UUID             `db:"thread_id"`
	MessageID uuid.UUID             `db:"message_id"`
	MemberID  uuid.UUID             `db:"member_id"`
	Status    MessageDeliveryStatus `db:"status"`
	Via       *string               `db:"via"`
	Error     map[string]any        `db:"error"`
	UpdatedAt time.Time             `db:"updated_at"`
}

// UnreadSummary is a participant's unread totals across all their chats.
type UnreadSummary struct {
	// Chats is the number of threads with at least one unread message.
	Chats int32 `db:"unread_chats"`
	// Messages is the total number of unread messages across all chats.
	Messages int64 `db:"unread_messages"`
}
