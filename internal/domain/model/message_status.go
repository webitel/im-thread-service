package model

import (
	"fmt"
	"maps"
	"regexp"
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

// Valid reports whether s is a receipt an external provider may report.
// SENT is excluded: it is assigned by this service on accept, never received.
func (s MessageDeliveryStatus) Valid() bool {
	switch s {
	case MessageDeliveryStatusDelivered, MessageDeliveryStatusRead, MessageDeliveryStatusFailed:
		return true
	case MessageDeliveryStatusUnspecified, MessageDeliveryStatusSent:
		return false
	default:
		return false
	}
}

// Normalized delivery error codes stored in message_errors.error->>'code'; the
// full list (most of them set by im-providers) is documented on DeliveryError
// in the message history proto.
const (
	DeliveryErrorFileTooLarge         = "file_too_large"
	DeliveryErrorFileUnavailable      = "file_unavailable"
	DeliveryErrorUnsupportedMedia     = "unsupported_media"
	DeliveryErrorRecipientUnavailable = "recipient_unavailable"
	DeliveryErrorChannelUnavailable   = "channel_unavailable"
	DeliveryErrorSendFailed           = "send_failed"
)

// NormalizeDeliveryErrorCode maps a stored or reported error code onto the
// normalized set; anything else (legacy raw provider codes such as "403" or
// "131047", or no code at all) becomes send_failed.
func NormalizeDeliveryErrorCode(code string) string {
	switch code {
	case DeliveryErrorFileTooLarge, DeliveryErrorFileUnavailable, DeliveryErrorUnsupportedMedia,
		DeliveryErrorRecipientUnavailable, DeliveryErrorChannelUnavailable, DeliveryErrorSendFailed:
		return code
	default:
		return DeliveryErrorSendFailed
	}
}

// SanitizeDeliveryError prepares failure details for storage: the code is
// normalized (a raw one is kept under provider_code) and secrets are redacted
// from the message. The input map is not modified.
func SanitizeDeliveryError(details map[string]any) map[string]any {
	out := make(map[string]any, len(details)+1)
	maps.Copy(out, details)

	raw := ""
	if v, ok := details["code"]; ok && v != nil {
		raw = fmt.Sprint(v)
	}

	out["code"] = NormalizeDeliveryErrorCode(raw)
	if raw != "" && raw != out["code"] {
		out["provider_code"] = raw
	}

	if msg, ok := details["message"].(string); ok {
		out["message"] = RedactSecrets(msg)
	}

	return out
}

const redacted = "[REDACTED]"

var secretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Telegram Bot API carries the bot token in the URL path: /bot<id>:<secret>/method.
	{regexp.MustCompile(`(?i)/bot\d+:[\w-]+`), "/bot" + redacted},
	// A bare Telegram-style token outside a URL.
	{regexp.MustCompile(`\b\d{6,}:[\w-]{30,}`), redacted},
	// Credentials in a URL's userinfo: scheme://user:password@host.
	{regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]+@`), "${1}" + redacted + "@"},
	// Credentials passed as query parameters.
	{regexp.MustCompile(`(?i)([?&](?:access_token|token|api_key|apikey|key|secret|client_secret|password|signature|sig)=)[^&#\s"']+`), "${1}" + redacted},
	// Authorization header values echoed back in an error.
	{regexp.MustCompile(`(?i)\b(bearer|basic)\s+[\w.~+/=-]+`), "${1} " + redacted},
}

// RedactSecrets strips credentials that channel SDKs tend to echo in error
// text (request URLs with tokens, userinfo, auth headers).
func RedactSecrets(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}

	return s
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

// StatusReceipt is a delivery/failure confirmation for a single message
// and recipient, reported by im-delivery or im-providers.
// For watermark receipts (UpToMessageID set), MessageID must be zero.
type StatusReceipt struct {
	DomainID      int32
	ThreadID      uuid.UUID
	MessageID     uuid.UUID
	UpToMessageID uuid.UUID
	UpToSeq       int64
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
	UpToSeq       int64
	// At is the confirmation time; zero means "now".
	At time.Time
	// Via is the confirmation source: ws|push|provider|bot.
	Via string
}

// StatusChange is a status row that was actually changed by an upsert.
// Duplicate and out-of-order receipts produce no changes.
// For watermark changes, MessageID is zero and UpToMessageID is set.
type StatusChange struct {
	DomainID      int32                 `db:"domain_id"`
	ThreadID      uuid.UUID             `db:"thread_id"`
	MessageID     uuid.UUID             `db:"message_id"`
	UpToMessageID uuid.UUID             `db:"up_to_message_id"`
	UpToSeq       int64                 `db:"up_to_seq"`
	MemberID      uuid.UUID             `db:"member_id"`
	Status        MessageDeliveryStatus `db:"status"`
	Via           *string               `db:"via"`
	Error         map[string]any        `db:"error"`
	UpdatedAt     time.Time             `db:"updated_at"`
	// UnreadCount is the member's unread messages in the thread after a read; nil otherwise.
	UnreadCount *int64 `db:"-"`
}

// UnreadSummary is a participant's unread totals across all their chats.
type UnreadSummary struct {
	// Chats is the number of threads with at least one unread message.
	Chats int32 `db:"unread_chats"`
	// Messages is the total number of unread messages across all chats.
	Messages int64 `db:"unread_messages"`
}
