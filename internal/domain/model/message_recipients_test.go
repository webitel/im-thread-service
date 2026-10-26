package model

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func TestMessage_RecipientIDs_ExcludesEffectiveSender(t *testing.T) {
	agent, customer, bot, sendAs := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name string
		msg  *Message
		want []uuid.UUID
	}{
		{
			name: "own message reaches others only",
			msg:  &Message{From: shared.Peer{ID: agent}, To: []*ThreadDialog{{ContactID: agent}, {ContactID: customer}, {ContactID: bot, IsBot: true}}},
			want: []uuid.UUID{customer, bot},
		},
		{
			name: "send-as sender is the effective one",
			msg:  &Message{From: shared.Peer{ID: agent}, SendAs: &sendAs, To: []*ThreadDialog{{ContactID: agent}, {ContactID: sendAs}, nil}},
			want: []uuid.UUID{agent},
		},
		{
			name: "no members",
			msg:  &Message{From: shared.Peer{ID: agent}},
			want: []uuid.UUID{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.msg.RecipientIDs(); !slices.Equal(got, tt.want) {
				t.Errorf("RecipientIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}
