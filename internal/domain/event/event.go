package event

import "github.com/google/uuid"

type Base interface {
	Outboxer

	Topic() string
}

// JournalEvent is a thread-scoped mutation written to the update journal for GetUpdates.
type JournalEvent interface {
	Outboxer

	// JournalThreadID is the thread the journal row belongs to.
	JournalThreadID() uuid.UUID
	// SetUpdatesCursor stamps the GetUpdates position before the event is serialized.
	SetUpdatesCursor(cursor string)
}

// Journaled carries the event's transaction as the recipients' GetUpdates cursor, so a
// client can resume catch-up from the latest live event it received.
type Journaled struct {
	UpdatesCursor string `json:"updates_cursor,omitempty"`
}

func (j *Journaled) SetUpdatesCursor(cursor string) { j.UpdatesCursor = cursor }
