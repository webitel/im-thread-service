package journal

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/event"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)

	return b
}

func TestProjectEvent(t *testing.T) {
	msgID, threadID, actor := uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name       string
		eventType  string
		payload    any
		wantOK     bool
		wantKind   string
		wantFields map[string]any
	}{
		{
			name:       "message created keeps only references",
			eventType:  event.MessageCreatedEvent,
			payload:    &event.MessageCreated{MessageID: msgID, ThreadID: threadID, From: &event.ThreadMember{ContactID: actor}, Body: "secret body"},
			wantOK:     true,
			wantKind:   KindMessageNew,
			wantFields: map[string]any{FieldMsgID: msgID.String(), FieldActor: actor.String()},
		},
		{
			name:       "message edited",
			eventType:  event.MessageEditedEvent,
			payload:    &event.MessageEdited{MessageID: msgID, ThreadID: threadID, EditedBy: &event.ThreadMember{ContactID: actor}, Body: "new"},
			wantOK:     true,
			wantKind:   KindMessageEdited,
			wantFields: map[string]any{FieldMsgID: msgID.String(), FieldActor: actor.String()},
		},
		{
			name:       "message deleted",
			eventType:  event.MessageDeletedEvent,
			payload:    &event.MessageDeleted{MessageID: msgID, ThreadID: threadID, DeletedBy: &event.Member{Contact: &event.MemberContact{ID: actor.String()}}},
			wantOK:     true,
			wantKind:   KindMessageDeleted,
			wantFields: map[string]any{FieldMsgID: msgID.String(), FieldActor: actor.String()},
		},
		{
			name:      "message reaction keeps the delta",
			eventType: event.MessageReactionEvent,
			payload:   &event.MessageReaction{MessageID: msgID, ThreadID: threadID, Reactor: &event.ThreadMember{ContactID: actor}, Emoji: "👍", Action: event.ReactionActionSet},
			wantOK:    true,
			wantKind:  KindMessageReaction,
			wantFields: map[string]any{
				FieldMsgID: msgID.String(), FieldActor: actor.String(), FieldEmoji: "👍", FieldAction: event.ReactionActionSet,
			},
		},
		{
			name:       "member joined",
			eventType:  event.MemberJoinedEvent,
			payload:    &event.MemberJoined{ThreadID: threadID, ContactID: actor},
			wantOK:     true,
			wantKind:   KindMemberChanged,
			wantFields: map[string]any{FieldContactID: actor.String(), FieldActor: actor.String(), FieldAction: ActionJoined},
		},
		{
			name:       "member left",
			eventType:  event.MemberLeftEvent,
			payload:    &event.MemberLeft{ThreadID: threadID, ContactID: actor},
			wantOK:     true,
			wantKind:   KindMemberChanged,
			wantFields: map[string]any{FieldContactID: actor.String(), FieldActor: actor.String(), FieldAction: ActionLeft},
		},
		{
			name:      "unknown events are not journaled",
			eventType: "im.typing.started",
			payload:   map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd, ok, err := ProjectEvent(tt.eventType, mustJSON(t, tt.payload))
			require.NoError(t, err)
			require.Equal(t, tt.wantOK, ok)

			if !ok {
				return
			}

			assert.Equal(t, tt.wantKind, upd.Kind)
			assert.Equal(t, threadID.String(), upd.ThreadID)
			assert.Equal(t, tt.wantFields, upd.Fields)
		})
	}
}

func TestProjectEvent_BadPayload(t *testing.T) {
	_, ok, err := ProjectEvent(event.MessageCreatedEvent, []byte(`{`))
	require.Error(t, err)
	assert.False(t, ok)
}

// Publish writes the journal row synchronously, so every JournalEvent must
// project from its own outbox payload; a miss would never reach GetUpdates.
func TestProjectEvent_EveryJournalEventProjects(t *testing.T) {
	thread := uuid.New()
	events := []event.JournalEvent{
		&event.MessageCreated{MessageID: uuid.New(), ThreadID: thread},
		&event.MessageEdited{MessageID: uuid.New(), ThreadID: thread},
		&event.MessageDeleted{MessageID: uuid.New(), ThreadID: thread},
		&event.MessageReaction{MessageID: uuid.New(), ThreadID: thread},
		&event.MemberJoined{ThreadID: thread, ContactID: uuid.New()},
		&event.MemberLeft{ThreadID: thread, ContactID: uuid.New()},
		&event.ThreadCreated{ID: thread, Recipient: &event.Recipient{ID: uuid.New()}},
		&event.MessageStatusChanged{ThreadID: thread, MemberID: uuid.New(), Status: event.StatusRead, UpToSeq: 4},
	}

	for _, e := range events {
		ev, err := e.ToOutbox()
		require.NoError(t, err)

		upd, ok, err := ProjectEvent(ev.Metadata["event_type"], ev.Payload)
		require.NoError(t, err, "%T", e)
		require.True(t, ok, "%T has no projection", e)
		assert.Equal(t, thread.String(), upd.ThreadID, "%T", e)
	}
}

// Every receipt is journaled: horizons for delivered/read, the message and reason for a failure.
func TestProjectEvent_StatusKinds(t *testing.T) {
	msg := uuid.New()

	for status, want := range map[string]string{
		event.StatusRead: KindReadChanged, event.StatusDelivered: KindDeliveredChanged, event.StatusFailed: KindMessageFailed,
	} {
		ev, err := (&event.MessageStatusChanged{
			ThreadID: uuid.New(), MemberID: uuid.New(), Status: status, UpToSeq: 4, MessageIDs: []uuid.UUID{msg},
			Error: map[string]any{"code": "131047", "message": "re-engagement window expired"},
		}).ToOutbox()
		require.NoError(t, err)

		upd, ok, err := ProjectEvent(ev.Metadata["event_type"], ev.Payload)
		require.NoError(t, err)
		require.True(t, ok, status)
		assert.Equal(t, want, upd.Kind, status)

		if status == event.StatusFailed {
			assert.Equal(t, msg.String(), upd.Fields[FieldMsgID])
			assert.Equal(t, "131047", upd.Fields[FieldErrCode])
			assert.Equal(t, "re-engagement window expired", upd.Fields[FieldErrMsg])
		}
	}
}
