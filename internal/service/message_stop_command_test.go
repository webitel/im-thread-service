package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// fakeThreadManager records ReleaseBotControl and counts EnsureDirectThread calls.
type fakeThreadManager struct {
	ensureCalls        int
	releaseCalls       int
	lastReleaseRequest *dto.ReleaseBotControlRequest
	ensureResult       *model.Thread
}

func (f *fakeThreadManager) EnsureDirectThread(_ context.Context, _ *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	f.ensureCalls++

	return f.ensureResult, nil
}

func (f *fakeThreadManager) ReleaseBotControl(_ context.Context, req *dto.ReleaseBotControlRequest) error {
	f.releaseCalls++
	f.lastReleaseRequest = req

	return nil
}

var _ ThreadManager = (*fakeThreadManager)(nil)

func senderRequest(from uuid.UUID) *dto.SendTextRequest {
	return &dto.SendTextRequest{From: shared.Peer{ID: from}}
}

func TestCanStopBot(t *testing.T) {
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

	svc := NewCommandService(nil, nil, nil)

	t.Run("user stops active bot", func(t *testing.T) {
		require.True(t, svc.canStopBot(newCommandRequest(threadWithBot(), senderRequest(userContactID))))
	})

	t.Run("no active bot controller", func(t *testing.T) {
		thread := threadWithBot()
		thread.BotControllerID = nil
		require.False(t, svc.canStopBot(newCommandRequest(thread, senderRequest(userContactID))))
	})

	t.Run("sender is a bot", func(t *testing.T) {
		require.False(t, svc.canStopBot(newCommandRequest(threadWithBot(), senderRequest(botContactID))))
	})

	t.Run("nil thread", func(t *testing.T) {
		require.False(t, svc.canStopBot(newCommandRequest(nil, senderRequest(userContactID))))
	})

	t.Run("sender not a member still stops bot", func(t *testing.T) {
		// An external sender with no membership row may still issue /close.
		require.True(t, svc.canStopBot(newCommandRequest(threadWithBot(), senderRequest(uuid.New()))))
	})
}

func TestHandleBotStopCommand_ReleasesBotAndPersistsConfirmation(t *testing.T) {
	threadID := uuid.New()
	userContactID := uuid.New()
	botContactID := uuid.New()
	botMemberID := uuid.New()
	userMemberID := uuid.New()

	thread := &model.Thread{
		ID:              threadID,
		BotControllerID: &botMemberID,
		Members: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: userMemberID}, ContactID: userContactID, IsBot: false},
			{BaseModel: shared.BaseModel{ID: botMemberID}, ContactID: botContactID, IsBot: true},
		},
	}

	threader := &fakeThreadManager{}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutboxStore{}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	messenger := &MessageService{
		logger: logger,
		uow: fakeUnitOfWork{
			messageStore: messageStore,
			outboxStore:  outboxStore,
		},
	}

	svc := NewCommandService(threader, messenger, logger)

	in := &dto.SendTextRequest{
		From:     shared.Peer{ID: userContactID},
		To:       shared.Peer{ID: botContactID},
		Body:     model.CommandClose.String(),
		DomainID: 1,
	}

	resp, err := svc.handleBotStopCommand(context.Background(), newCommandRequest(thread, in))
	require.NoError(t, err)

	// Bot control is released for this thread on behalf of the initiating user member.
	require.Equal(t, 1, threader.releaseCalls)
	require.Equal(t, threadID, threader.lastReleaseRequest.ThreadID)
	require.Equal(t, userMemberID, threader.lastReleaseRequest.InitiatorMemberID)

	// The confirmation is saved directly, without re-resolving the thread — otherwise
	// EnsureDirectThread would re-arm and restart the bot we just released.
	require.Equal(t, 0, threader.ensureCalls, "must not re-run EnsureDirectThread")

	saved := messageStore.lastSavedSystemMessage
	require.NotNil(t, saved, "bot_stopped confirmation must be persisted")
	require.Equal(t, threadID, saved.ThreadID)
	require.Equal(t, model.MessageTypeSystem, saved.Type)
	require.Equal(t, botStoppedSystemType, saved.System.Type)
	require.Equal(t, model.CommandClose.String(), saved.Body)
	// messages.metadata is JSONB NOT NULL; a nil map fails the insert and nothing shows in chat.
	require.NotNil(t, saved.Metadata, "messages.metadata is NOT NULL")

	// The confirmation must reach the user but never the bot (a bot recipient restarts the flow).
	require.NotEmpty(t, saved.To)

	for _, m := range saved.To {
		require.False(t, m.IsBot, "bot must not be a recipient of the bot_stopped confirmation")
	}

	require.Equal(t, saved.ID, resp.ID)
}
