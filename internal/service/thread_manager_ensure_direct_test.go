package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type ensureDirectThreadStore struct {
	fakeThreadStore

	resolved *model.Thread
}

func (f *ensureDirectThreadStore) ResolveThread(context.Context, model.ResolveThreadQuery) (*model.Thread, error) {
	return f.resolved, nil
}

func (f *ensureDirectThreadStore) Create(_ context.Context, thread *model.Thread) (*model.Thread, error) {
	thread.ID = uuid.New()

	return thread, nil
}

type botLookupContactInfo struct {
	fakeContactInfo

	isBot bool
	calls int
}

func (f *botLookupContactInfo) IsBot(context.Context, uuid.UUID, int) (bool, error) {
	f.calls++

	return f.isBot, nil
}

func directPeer(name string) *shared.Peer {
	return &shared.Peer{ID: uuid.New(), Type: shared.PeerContact, Identity: &shared.Identity{Name: name}}
}

func TestEnsureDirectThread_NewThread(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		peerIsBot bool
	}{
		{name: "peer is bot", peerIsBot: true},
		{name: "peer is not bot", peerIsBot: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			contactInfo := &botLookupContactInfo{isBot: tt.peerIsBot}
			botControl := &fakeBotControlStore{}
			dialogs := &fakeThreadDialogStore{}
			outbox := &fakeOutboxStore{}

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadStore:       &ensureDirectThreadStore{},
					threadDialogStore: dialogs,
					outboxStore:       outbox,
					botControlStore:   botControl,
				},
				privacyChecker: fakePrivacyChecker{},
				contactInfo:    contactInfo,
			}

			thread, err := svc.EnsureDirectThread(context.Background(), &dto.EnsureDirectThreadRequest{
				DomainID: 1,
				From:     directPeer("customer"),
				To:       directPeer("peer"),
			})
			require.NoError(t, err)
			require.Equal(t, 1, contactInfo.calls)
			require.Equal(t, tt.peerIsBot, dialogs.lastCreate.IsBot)

			if !tt.peerIsBot {
				require.Zero(t, botControl.pushCalls)
				require.Nil(t, thread.BotControllerID)
				require.Nil(t, findGrantedEvent(outbox))

				return
			}

			require.Equal(t, 1, botControl.pushCalls)
			require.Equal(t, model.BotControlReasonInitial, botControl.lastPushTransition.Reason)
			require.Equal(t, dialogs.lastCreate.ID, botControl.lastPushTransition.NewMemberID)
			require.NotNil(t, thread.BotControllerID)
			require.Equal(t, dialogs.lastCreate.ID, *thread.BotControllerID)
			require.NotNil(t, findGrantedEvent(outbox))
		})
	}
}

func TestEnsureDirectThread_ExistingThread(t *testing.T) {
	t.Parallel()

	customer := directPeer("customer")
	bot := directPeer("bot")
	human := directPeer("agent")
	threadPeer := &shared.Peer{ID: uuid.New(), Type: shared.PeerThread}
	botMemberID := uuid.New()

	botThread := func() []*model.ThreadDialog {
		return []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: customer.ID, ThreadRole: model.RoleOwner},
			{BaseModel: shared.BaseModel{ID: botMemberID}, ContactID: bot.ID, ThreadRole: model.RoleOwner, IsBot: true},
		}
	}

	tests := []struct {
		name      string
		members   []*model.ThreadDialog
		from, to  *shared.Peer
		wantRearm bool
	}{
		{name: "message to bot re-arms it", members: botThread(), from: customer, to: bot, wantRearm: true},
		{name: "message from bot does not re-arm it", members: botThread(), from: bot, to: customer},
		{name: "message addressed to thread does not re-arm bot", members: botThread(), from: customer, to: threadPeer},
		{
			name: "thread without bot stays untouched",
			members: []*model.ThreadDialog{
				{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: customer.ID, ThreadRole: model.RoleOwner},
				{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: human.ID, ThreadRole: model.RoleOwner},
			},
			from: customer,
			to:   human,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			contactInfo := &botLookupContactInfo{isBot: true}
			botControl := &fakeBotControlStore{}
			outbox := &fakeOutboxStore{}
			existing := &model.Thread{DomainID: 1, Kind: model.ThreadDirect, Members: tt.members}
			existing.ID = uuid.New()

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadStore:       &ensureDirectThreadStore{resolved: existing},
					threadDialogStore: &fakeThreadDialogStore{},
					outboxStore:       outbox,
					botControlStore:   botControl,
				},
				privacyChecker: fakePrivacyChecker{},
				contactInfo:    contactInfo,
			}

			thread, err := svc.EnsureDirectThread(context.Background(), &dto.EnsureDirectThreadRequest{
				DomainID: 1,
				From:     tt.from,
				To:       tt.to,
			})
			require.NoError(t, err)
			require.Zero(t, contactInfo.calls)

			if !tt.wantRearm {
				require.Zero(t, botControl.pushCalls)
				require.Nil(t, thread.BotControllerID)
				require.Nil(t, findGrantedEvent(outbox))

				return
			}

			require.Equal(t, 1, botControl.pushCalls)
			require.Equal(t, &botMemberID, thread.BotControllerID)
			require.NotNil(t, findGrantedEvent(outbox))
		})
	}
}
