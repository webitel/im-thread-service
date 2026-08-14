package event

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const (
	// TypingEvent is the logical name of the ephemeral typing indicator.
	TypingEvent = "im.thread.typing"

	// TypingVersion is the wire version of the typing payload.
	TypingVersion = "v1"
)

// Typing is an ephemeral "…is typing" indicator (optionally carrying a live
// draft preview). Unlike every other event in this service it is NOT an
// Outboxer: it is published fire-and-forget straight to the broker, never
// written to the transactional outbox, never persisted and never replayed.
//
// The server holds no typing state — the payload carries TimeoutMS and clients
// self-expire the indicator (Telegram model).
type Typing struct {
	ThreadID   uuid.UUID `json:"thread_id"`
	MemberID   uuid.UUID `json:"member_id"`
	Role       int32     `json:"role"`
	TimeoutMS  int32     `json:"timeout_ms"`
	OccurredAt time.Time `json:"occurred_at"`

	// To are the recipient member ids for the indicator (the thread's internal
	// participants except the sender). Delivery fans the indicator out to their
	// online sessions.
	To []uuid.UUID `json:"to,omitempty"`

	// DomainID lets im-delivery enrich the typing sender (name/type) via its
	// contact resolver — exactly like a message sender — instead of the sender
	// name being carried on the event.
	DomainID int32 `json:"domain_id"`

	// PreviewText is the sender's unsent draft (Live Typing Preview).
	// PRIVACY: present only when the preview feature is on and there is at least
	// one authorized recipient. It is never logged (see LogValue) and never
	// persisted anywhere on the path.
	PreviewText string `json:"preview_text,omitempty"`

	// PreviewVisibleTo lists the member ids allowed to receive PreviewText.
	// Resolved server-side by role; empty means no preview.
	PreviewVisibleTo []uuid.UUID `json:"preview_visible_to,omitempty"`
}

// TypingTopic is the ephemeral routing key for a thread's typing events.
// It mirrors the message-event topic shape (im_message.<recipient>.<kind>.<ver>)
// so delivery can bind both with a single im_message.<thread>.* pattern.
func TypingTopic(threadID uuid.UUID) string {
	return fmt.Sprintf("im_message.%s.typing.%s", threadID, TypingVersion)
}

// LogValue implements slog.LogValuer so the draft can never leak into logs even
// if the whole event is logged: PreviewText is replaced by its byte length.
func (t Typing) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("thread_id", t.ThreadID.String()),
		slog.String("member_id", t.MemberID.String()),
		slog.Int("timeout_ms", int(t.TimeoutMS)),
		slog.Int("preview_bytes", len(t.PreviewText)),
		slog.Int("preview_recipients", len(t.PreviewVisibleTo)),
	)
}
