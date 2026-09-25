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
}
