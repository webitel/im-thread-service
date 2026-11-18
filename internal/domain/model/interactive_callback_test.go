package model

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/event"
)

func TestInteractiveCallbackCreatedEventCarriesMembers(t *testing.T) {
	t.Parallel()

	reactorContact, reactorMember := uuid.New(), uuid.New()
	receiverContact, receiverMember := uuid.New(), uuid.New()

	tests := []struct {
		name         string
		callback     InteractiveCallback
		wantReactor  *event.ThreadMember
		wantReceiver *event.ThreadMember
	}{
		{
			name: "both memberships resolved",
			callback: InteractiveCallback{
				ReactedBy:         reactorContact,
				Receiver:          receiverContact,
				ReactedByMemberID: &reactorMember,
				ReactedByRole:     ThreadRole(1),
				ReceiverMemberID:  &receiverMember,
				ReceiverRole:      ThreadRole(2),
				ReceiverIsBot:     true,
			},
			wantReactor:  &event.ThreadMember{ID: &reactorMember, ContactID: reactorContact, Role: 1},
			wantReceiver: &event.ThreadMember{ID: &receiverMember, ContactID: receiverContact, Role: 2, IsBot: true},
		},
		{
			name: "receiver without membership keeps its contact id",
			callback: InteractiveCallback{
				ReactedBy:         reactorContact,
				Receiver:          receiverContact,
				ReactedByMemberID: &reactorMember,
				ReactedByRole:     ThreadRole(1),
			},
			wantReactor:  &event.ThreadMember{ID: &reactorMember, ContactID: reactorContact, Role: 1},
			wantReceiver: &event.ThreadMember{ContactID: receiverContact},
		},
		{
			name: "nil member id is treated as no membership",
			callback: InteractiveCallback{
				ReactedBy:         reactorContact,
				ReactedByMemberID: &uuid.Nil,
			},
			wantReactor: &event.ThreadMember{ContactID: reactorContact},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cb := tt.callback
			evs := cb.WithCreatedEvent(context.Background()).Events()
			require.Len(t, evs, 1)

			ev, ok := evs[0].(*event.InteractiveCallbackPayload)
			require.True(t, ok)
			require.Equal(t, tt.wantReactor, ev.ReactedByMember)
			require.Equal(t, tt.wantReceiver, ev.ReceiverMember)
		})
	}
}

// TestInteractiveCallbackEventWireShape pins the outbox JSON: the member blocks
// are additive and the legacy contact-id fields keep their names and types.
func TestInteractiveCallbackEventWireShape(t *testing.T) {
	t.Parallel()

	reactorContact, reactorMember := uuid.New(), uuid.New()
	receiverContact := uuid.New()
	reactedAt := time.Date(2026, 11, 17, 14, 40, 55, 0, time.UTC)

	cb := &InteractiveCallback{
		ReactedBy:         reactorContact,
		InReplyTo:         uuid.New(),
		ButtonCode:        "btn_code",
		CallbackData:      "btn_data",
		ReactedAt:         reactedAt,
		ThreadID:          uuid.New(),
		DomainID:          1,
		Receiver:          receiverContact,
		ReactedByMemberID: &reactorMember,
		ReactedByRole:     ThreadRole(1),
	}

	out, err := cb.WithCreatedEvent(context.Background()).Events()[0].ToOutbox()
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Payload, &got))

	require.Equal(t, receiverContact.String(), got["receiver"])
	require.Equal(t, reactorContact.String(), got["reacted_by"].(map[string]any)["id"])
	require.Equal(t, "2026-11-17T14:40:55Z", got["reacted_at"])

	require.Equal(t, map[string]any{
		"member_id":  reactorMember.String(),
		"contact_id": reactorContact.String(),
		"role":       float64(1),
		"is_bot":     false,
	}, got["reacted_by_member"])
	require.Equal(t, map[string]any{
		"contact_id": receiverContact.String(),
		"role":       float64(0),
		"is_bot":     false,
	}, got["receiver_member"])
}
