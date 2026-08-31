package model

import (
	"testing"

	"github.com/google/uuid"
)

func strPtr(s string) *string { return &s }

func TestExtractExternalPeers(t *testing.T) {
	gate := uuid.New()
	human := uuid.New()
	bot := uuid.New()

	tests := []struct {
		name    string
		dialogs ThreadDialogs
		wantIDs []uuid.UUID
	}{
		{
			name:    "empty",
			dialogs: nil,
			wantIDs: nil,
		},
		{
			name: "human with via is external, bot without via is not",
			dialogs: ThreadDialogs{
				{ContactID: human, Via: strPtr(gate.String())},
				{ContactID: bot, IsBot: true},
			},
			wantIDs: []uuid.UUID{human},
		},
		{
			name: "bot with a stale via is never external",
			dialogs: ThreadDialogs{
				{ContactID: human, Via: strPtr(gate.String())},
				{ContactID: bot, IsBot: true, Via: strPtr(gate.String())},
			},
			wantIDs: []uuid.UUID{human},
		},
		{
			name: "no external when only a bot carries via and no human present",
			dialogs: ThreadDialogs{
				{ContactID: bot, IsBot: true, Via: strPtr(gate.String())},
			},
			wantIDs: nil,
		},
		{
			name: "self-heal: human lost via, gate id recovered from bot row",
			dialogs: ThreadDialogs{
				{ContactID: human},
				{ContactID: bot, IsBot: true, Via: strPtr(gate.String())},
			},
			wantIDs: []uuid.UUID{human},
		},
		{
			name: "no self-heal in group threads: two non-bots without via stay external-less",
			dialogs: ThreadDialogs{
				{ContactID: human},
				{ContactID: uuid.New()},
				{ContactID: bot, IsBot: true, Via: strPtr(gate.String())},
			},
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dialogs.ExtractExternalPeers()
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d external peers, want %d", len(got), len(tt.wantIDs))
			}

			for i, want := range tt.wantIDs {
				if got[i].ContactID != want {
					t.Errorf("peer[%d].ContactID = %s, want %s", i, got[i].ContactID, want)
				}

				if got[i].Via != gate.String() {
					t.Errorf("peer[%d].Via = %s, want %s", i, got[i].Via, gate.String())
				}
			}
		})
	}
}
