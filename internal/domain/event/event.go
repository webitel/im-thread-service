package event

import "github.com/google/uuid"

type Base interface {
	Outboxer

	Topic() string
}

// JournalEvent is a thread-scoped mutation participating in the update journal.
// Its update_seq is the catch-up cursor and rides on both live event and journal row.
type JournalEvent interface {
	Outboxer

	// JournalThreadID is the thread whose update_seq counter this event advances.
	JournalThreadID() uuid.UUID
	// SetUpdateSeq records the stamped sequence before the event is serialized.
	SetUpdateSeq(seq int64)
}
