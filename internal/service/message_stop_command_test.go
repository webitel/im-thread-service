package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func TestIsStopCommand(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"exact", "/close", true},
		{"trimmed", "  /close  ", true},
		{"trailing newline", "/close\n", true},
		{"prefix only", "/close please", false},
		{"different command", "/stop", false},
		{"empty", "", false},
		{"contains close", "please /close", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isStopCommand(tc.body))
		})
	}
}

func TestShouldStopBot(t *testing.T) {
	botMemberID := uuid.New()
	userContactID := uuid.New()
	botContactID := uuid.New()

	threadWithBot := func() *model.Thread {
		return &model.Thread{
			ID:              uuid.New(),
			BotControllerID: &botMemberID,
			Members: []*model.ThreadDialog{
				{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: userContactID, IsBot: false},
				{BaseModel: shared.BaseModel{ID: botMemberID}, ContactID: botContactID, IsBot: true},
			},
		}
	}

	svc := &MessageService{}

	t.Run("user stops active bot", func(t *testing.T) {
		require.True(t, svc.shouldStopBot(threadWithBot(), userContactID))
	})

	t.Run("no active bot controller", func(t *testing.T) {
		thread := threadWithBot()
		thread.BotControllerID = nil
		require.False(t, svc.shouldStopBot(thread, userContactID))
	})

	t.Run("sender is a bot", func(t *testing.T) {
		require.False(t, svc.shouldStopBot(threadWithBot(), botContactID))
	})

	t.Run("nil thread", func(t *testing.T) {
		require.False(t, svc.shouldStopBot(nil, userContactID))
	})

	t.Run("sender not a member still stops bot", func(t *testing.T) {
		// An external sender with no membership row may still issue /close.
		require.True(t, svc.shouldStopBot(threadWithBot(), uuid.New()))
	})
}
