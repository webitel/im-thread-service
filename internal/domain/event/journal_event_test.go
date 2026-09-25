package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJournalEventMembership validates which events must be journaled for GetUpdates.
func TestJournalEventMembership(t *testing.T) {
	tests := []struct {
		name  string
		event Outboxer
		want  bool
	}{
		{name: "message created", event: &MessageCreated{}, want: true},
		{name: "message edited", event: &MessageEdited{}, want: true},
		{name: "message deleted", event: &MessageDeleted{}, want: true},
		{name: "message reaction", event: &MessageReaction{}, want: true},
		{name: "member joined", event: &MemberJoined{}, want: true},
		{name: "member left", event: &MemberLeft{}, want: true},
		{name: "thread created", event: &ThreadCreated{}, want: true},
		{name: "message status changed", event: &MessageStatusChanged{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.event.(JournalEvent)
			assert.Equal(t, tt.want, ok)
		})
	}
}
