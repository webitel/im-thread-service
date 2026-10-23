package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
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

// commitFailingUnitOfWork runs the transaction body and then reports a commit failure.
type commitFailingUnitOfWork struct {
	fakeUnitOfWork
}

func (f commitFailingUnitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context, store.UnitOfWork) error) error {
	if err := fn(ctx, f); err != nil {
		return err
	}

	return errCommitFailed
}

var errCommitFailed = errors.New("commit failed")

func viaInfoPeer(name, via, sourceName, sourceType string) *shared.Peer {
	p := directPeer(name)
	p.Identity.ViaInfo = &shared.ViaInfo{ID: via, Name: sourceName, Type: sourceType}

	return p
}

func newEnsureDirectService(uow fakeUnitOfWork, refs *ReferenceCache, contactInfo ContactInfoProvider, existing *model.Thread) *ThreadManagementService {
	uow.threadStore = &ensureDirectThreadStore{resolved: existing}

	return &ThreadManagementService{
		uow:            uow,
		privacyChecker: fakePrivacyChecker{},
		contactInfo:    contactInfo,
		refs:           refs,
	}
}

func TestEnsureDirectThread_NewThreadWritesSourceInCreationTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	refs := newTestReferenceCache()
	uow := fakeUnitOfWork{
		threadDialogStore: &fakeThreadDialogStore{},
		outboxStore:       &fakeOutboxStore{},
		botControlStore:   &fakeBotControlStore{},
		referenceStore:    refStore,
	}
	from := viaInfoPeer("customer", "gate-1", "Support", "telegram")
	to := directPeer("peer")

	svc := newEnsureDirectService(uow, refs, &botLookupContactInfo{}, nil)
	_, err := svc.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{DomainID: 1, From: from, To: to})
	require.NoError(t, err)

	require.Equal(t, []*model.SourceRef{{GateID: "gate-1", DomainID: 1, Name: "Support", Type: "telegram"}}, refStore.sources)
	require.False(t, refs.SourceChanged(ctx, refStore.sources[0]))

	// A repeated send to the now existing thread hits the cache and writes nothing.
	existing := &model.Thread{DomainID: 1, Kind: model.ThreadDirect}
	existing.ID = uuid.New()
	svc = newEnsureDirectService(uow, refs, &botLookupContactInfo{}, existing)
	_, err = svc.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{DomainID: 1, From: from, To: to})
	require.NoError(t, err)
	require.Len(t, refStore.sources, 1)
}

func TestEnsureDirectThread_ExistingThreadRefreshesSourceOnlyOnChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	refs := newTestReferenceCache()
	existing := &model.Thread{DomainID: 1, Kind: model.ThreadDirect}
	existing.ID = uuid.New()
	uow := fakeUnitOfWork{
		threadDialogStore: &fakeThreadDialogStore{},
		outboxStore:       &fakeOutboxStore{},
		botControlStore:   &fakeBotControlStore{},
		referenceStore:    refStore,
	}
	from := viaInfoPeer("customer", "gate-1", "Support", "telegram")
	to := directPeer("peer")
	req := &dto.EnsureDirectThreadRequest{DomainID: 1, From: from, To: to}

	// Cache miss: one upsert.
	svc := newEnsureDirectService(uow, refs, &botLookupContactInfo{}, existing)
	_, err := svc.EnsureDirectThread(ctx, req)
	require.NoError(t, err)
	require.Len(t, refStore.sources, 1)

	// Same value remembered: no upsert.
	_, err = svc.EnsureDirectThread(ctx, req)
	require.NoError(t, err)
	require.Len(t, refStore.sources, 1)

	// Renamed gate: one upsert with the new name.
	renamed := &dto.EnsureDirectThreadRequest{DomainID: 1, From: viaInfoPeer("customer", "gate-1", "Sales", "telegram"), To: to}
	_, err = svc.EnsureDirectThread(ctx, renamed)
	require.NoError(t, err)
	require.Len(t, refStore.sources, 2)
	require.Equal(t, "Sales", refStore.sources[1].Name)
}

func TestEnsureDirectThread_WithoutViaInfoWritesNoSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	refs := newTestReferenceCache()
	uow := fakeUnitOfWork{
		threadDialogStore: &fakeThreadDialogStore{},
		outboxStore:       &fakeOutboxStore{},
		botControlStore:   &fakeBotControlStore{},
		referenceStore:    refStore,
	}

	// A gate id without a name (old provider) and an empty name are both skipped.
	noInfo := directPeer("customer")
	noInfo.Identity.ViaInfo = &shared.ViaInfo{ID: "gate-1"}
	emptyName := viaInfoPeer("customer", "gate-2", "", "telegram")

	existing := &model.Thread{DomainID: 1, Kind: model.ThreadDirect}
	existing.ID = uuid.New()

	for _, from := range []*shared.Peer{noInfo, emptyName} {
		svc := newEnsureDirectService(uow, refs, &botLookupContactInfo{}, nil)
		thread, err := svc.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{DomainID: 1, From: from, To: directPeer("peer")})
		require.NoError(t, err)
		require.NotNil(t, thread)

		svc = newEnsureDirectService(uow, refs, &botLookupContactInfo{}, existing)
		_, err = svc.EnsureDirectThread(ctx, &dto.EnsureDirectThreadRequest{DomainID: 1, From: from, To: directPeer("peer")})
		require.NoError(t, err)
	}

	require.Empty(t, refStore.sources)
}

func TestEnsureDirectThread_SourceRefreshIsIndependentOfThreadCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	refs := newTestReferenceCache()
	base := fakeUnitOfWork{
		threadDialogStore: &fakeThreadDialogStore{},
		outboxStore:       &fakeOutboxStore{},
		botControlStore:   &fakeBotControlStore{},
		referenceStore:    refStore,
	}
	from := viaInfoPeer("customer", "gate-1", "Support", "telegram")
	req := &dto.EnsureDirectThreadRequest{DomainID: 1, From: from, To: directPeer("peer")}

	// The source is refreshed before the creation transaction, so a failed commit
	// does not undo it: the gate name is true regardless of the thread write.
	base.threadStore = &ensureDirectThreadStore{}
	svc := &ThreadManagementService{
		uow:            commitFailingUnitOfWork{fakeUnitOfWork: base},
		privacyChecker: fakePrivacyChecker{},
		contactInfo:    &botLookupContactInfo{},
		refs:           refs,
	}
	_, err := svc.EnsureDirectThread(ctx, req)
	require.ErrorIs(t, err, errCommitFailed)
	require.Len(t, refStore.sources, 1)
	require.False(t, refs.SourceChanged(ctx, refStore.sources[0]), "a written source is remembered")

	// The retry creates the thread and skips the unchanged source.
	svc = newEnsureDirectService(base, refs, &botLookupContactInfo{}, nil)
	_, err = svc.EnsureDirectThread(ctx, req)
	require.NoError(t, err)
	require.Len(t, refStore.sources, 1)
}
