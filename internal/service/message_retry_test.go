package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestReplaceRetried(t *testing.T) {
	t.Parallel()

	newReplacement := func() *model.Message {
		return &model.Message{
			ID:       uuid.New(),
			ThreadID: uuid.New(),
			DomainID: 1,
			From:     shared.Peer{ID: uuid.New(), Type: shared.PeerContact},
			Type:     model.MessageTypeText,
		}
	}

	t.Run("no plan leaves the store untouched", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{}
		statuses := &recordingMessageStatusStore{}
		svc := newDeleteTestService(msgStore, statuses)

		require.NoError(t, svc.replaceRetried(context.Background(), svc.uow, nil, newReplacement()))
		require.Zero(t, msgStore.replaceFailedCalls)
		require.Empty(t, statuses.recountedThreads)
	})

	t.Run("send_as: replaced on behalf of the real actor, not the shared identity", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{}
		statuses := &recordingMessageStatusStore{}
		outbox := &fakeOutboxStore{}
		svc := newDeleteTestService(msgStore, statuses)
		uow := svc.uow.(fakeUnitOfWork)
		uow.outboxStore = outbox

		// SaveMessage returns the send_as identity as the replacement's sender.
		replacement := newReplacement()
		agent := shared.Peer{ID: uuid.New(), Type: shared.PeerContact}
		plan := &retryPlan{originalID: uuid.New(), threadID: replacement.ThreadID, actor: agent}

		require.NoError(t, svc.replaceRetried(context.Background(), uow, plan, replacement))

		require.Equal(t, 1, msgStore.replaceFailedCalls)
		require.Equal(t, plan.originalID, msgStore.lastReplacedOriginal)
		require.Equal(t, replacement.ID, msgStore.lastReplacement)
		require.Equal(t, replacement.ThreadID, msgStore.lastReplaceThread)
		require.Equal(t, agent.ID, msgStore.lastReplaceActor)
		require.Equal(t, []uuid.UUID{replacement.ThreadID}, statuses.recountedThreads)

		require.Len(t, outbox.published, 1)
		deleted, ok := outbox.published[0].event.(*event.MessageDeleted)
		require.True(t, ok, "want MessageDeleted, got %T", outbox.published[0].event)
		require.Equal(t, plan.originalID, deleted.MessageID)
		require.NotNil(t, deleted.ReplacedBy)
		require.Equal(t, replacement.ID, *deleted.ReplacedBy)
		require.NotNil(t, deleted.DeletedBy)
		require.Equal(t, agent.ID.String(), deleted.DeletedBy.Contact.ID)
	})

	t.Run("a message that is no longer retryable aborts the send", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{replaceFailedErr: errNotRetryable()}
		statuses := &recordingMessageStatusStore{}
		svc := newDeleteTestService(msgStore, statuses)

		plan := &retryPlan{originalID: uuid.New(), actor: shared.Peer{ID: uuid.New()}}
		err := svc.replaceRetried(context.Background(), svc.uow, plan, newReplacement())
		require.Equal(t, codes.InvalidArgument, errors.Code(err))
		require.Empty(t, statuses.recountedThreads)
	})
}

func TestPlanRetry(t *testing.T) {
	t.Parallel()

	actor := shared.Peer{ID: uuid.New(), Type: shared.PeerContact}
	other := uuid.New()
	threadID := uuid.New()
	originalID := uuid.New()
	previousRetry := uuid.New()

	failed := func() *model.RetryTarget {
		return &model.RetryTarget{ID: originalID, ThreadID: threadID, AuthorID: actor.ID, Failed: true}
	}

	failedWith := func(mutate func(*model.RetryTarget)) *model.RetryTarget {
		t := failed()
		mutate(t)

		return t
	}

	tests := []struct {
		name       string
		target     *model.RetryTarget
		wantErr    bool
		wantReplay uuid.UUID
	}{
		{name: "own failed message", target: failed()},
		{name: "unknown message", wantErr: true},
		{name: "not failed", target: failedWith(func(t *model.RetryTarget) { t.Failed = false }), wantErr: true},
		{name: "deleted", target: failedWith(func(t *model.RetryTarget) { t.Deleted = true }), wantErr: true},
		{name: "other author", target: failedWith(func(t *model.RetryTarget) { t.AuthorID = other }), wantErr: true},
		{
			name: "already retried by the same actor replays",
			target: func() *model.RetryTarget {
				t := failed()
				t.Deleted, t.RetriedBy, t.RetriedByAuthor, t.RetriedByThread = true, &previousRetry, &actor.ID, &threadID

				return t
			}(),
			wantReplay: previousRetry,
		},
		{
			name: "already retried by someone else",
			target: func() *model.RetryTarget {
				t := failed()
				t.Deleted, t.RetriedBy, t.RetriedByAuthor, t.RetriedByThread = true, &previousRetry, &other, &threadID

				return t
			}(),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msgStore := &fakeMessageStore{retryTargets: map[uuid.UUID]*model.RetryTarget{}}
			if tc.target != nil {
				msgStore.retryTargets[originalID] = tc.target
			}

			svc := newDeleteTestService(msgStore, &recordingMessageStatusStore{})

			plan, err := svc.planRetry(context.Background(), &originalID, actor, 1)
			if tc.wantErr {
				require.Equal(t, codes.InvalidArgument, errors.Code(err))

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantReplay != uuid.Nil, plan.replay())
			require.Equal(t, tc.wantReplay, plan.replayID)
			require.Equal(t, threadID, plan.threadID)
			require.Equal(t, actor, plan.actor)
		})
	}

	t.Run("no retry_of is no plan", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{}
		svc := newDeleteTestService(msgStore, &recordingMessageStatusStore{})

		for _, retryOf := range []*uuid.UUID{nil, new(uuid.UUID)} {
			plan, err := svc.planRetry(context.Background(), retryOf, actor, 1)
			require.NoError(t, err)
			require.Nil(t, plan)
		}

		require.Zero(t, msgStore.retryTargetCalls)
	})
}

type countingThreadManager struct {
	thread *model.Thread
	calls  int
}

func (f *countingThreadManager) EnsureDirectThread(context.Context, *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	f.calls++

	return f.thread, nil
}

type countingProvidersAdapter struct {
	fakeProvidersAdapter

	sent []*model.Message
}

func (f *countingProvidersAdapter) SendMessage(_ context.Context, msg *model.Message) error {
	f.sent = append(f.sent, msg)

	return nil
}

type countingMediaProcessor struct{ processCalls int }

func (f *countingMediaProcessor) Process(context.Context, int64, []AttachmentProcessor) error {
	f.processCalls++

	return nil
}

func (f *countingMediaProcessor) FetchFileLinks(context.Context, int64, []AttachmentProcessor) <-chan fetchLinksResult {
	ch := make(chan fetchLinksResult)
	close(ch)

	return ch
}

func (f *countingMediaProcessor) FetchFileLinksWithMetadata(context.Context, int64, []AttachmentProcessor) (storageFilesResponse, error) {
	return storageFilesResponse{FilesMetadata: make(map[int64]*fileMetadata)}, nil
}

// retryFixture is a send pipeline with fakes that count every side effect.
type retryFixture struct {
	svc       *MessageService
	msgStore  *fakeMessageStore
	outbox    *fakeOutboxStore
	threader  *countingThreadManager
	providers *countingProvidersAdapter
	media     *countingMediaProcessor

	sender     shared.Peer
	recipient  shared.Peer
	thread     *model.Thread
	originalID uuid.UUID
}

func newRetryFixture() *retryFixture {
	f := &retryFixture{
		sender:     shared.Peer{ID: uuid.New(), Type: shared.PeerContact},
		recipient:  shared.Peer{ID: uuid.New(), Type: shared.PeerContact},
		originalID: uuid.New(),
	}

	f.thread = &model.Thread{ID: uuid.New(), Members: []*model.ThreadDialog{
		{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: f.sender.ID},
		{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: f.recipient.ID},
	}}

	f.msgStore = &fakeMessageStore{retryTargets: map[uuid.UUID]*model.RetryTarget{
		f.originalID: {ID: f.originalID, ThreadID: f.thread.ID, AuthorID: f.sender.ID, Failed: true},
	}}
	f.outbox = &fakeOutboxStore{}
	f.threader = &countingThreadManager{thread: f.thread}
	f.providers = &countingProvidersAdapter{}
	f.media = &countingMediaProcessor{}

	f.svc = &MessageService{
		tagHooks:         &fakeTagHooks{},
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		contactClient:    fakeContactDirectory{},
		threader:         f.threader,
		providersAdapter: f.providers,
		mediaProcessor:   f.media,
		uow: fakeUnitOfWork{
			messageStore:       f.msgStore,
			outboxStore:        f.outbox,
			threadDialogStore:  &fakeThreadDialogStore{},
			messageStatusStore: &recordingMessageStatusStore{},
		},
	}

	return f
}

func (f *retryFixture) target() *model.RetryTarget { return f.msgStore.retryTargets[f.originalID] }

func (f *retryFixture) markRetried(by uuid.UUID) {
	t := f.target()
	t.Deleted, t.RetriedBy, t.RetriedByAuthor, t.RetriedByThread = true, &by, &f.sender.ID, &f.thread.ID
}

// requireNoSideEffects asserts a rejected or replayed send touched nothing.
func (f *retryFixture) requireNoSideEffects(t *testing.T) {
	t.Helper()

	require.Empty(t, f.msgStore.savedMessages, "no message saved")
	require.Zero(t, f.msgStore.replaceFailedCalls)
	require.Empty(t, f.outbox.published, "no events")
	require.Empty(t, f.providers.sent, "provider not called")
	require.Zero(t, f.media.processCalls, "media not processed")
}

func (f *retryFixture) requireReplaced(t *testing.T, replacementID uuid.UUID) {
	t.Helper()

	require.Equal(t, 1, f.msgStore.replaceFailedCalls)
	require.Equal(t, f.originalID, f.msgStore.lastReplacedOriginal)
	require.Equal(t, replacementID, f.msgStore.lastReplacement)
	require.Equal(t, f.sender.ID, f.msgStore.lastReplaceActor)

	var replaced *event.MessageDeleted

	for _, p := range f.outbox.published {
		if e, ok := p.event.(*event.MessageDeleted); ok {
			replaced = e
		}
	}

	require.NotNil(t, replaced, "the original is announced deleted")
	require.Equal(t, f.originalID, replaced.MessageID)
	require.NotNil(t, replaced.ReplacedBy)
	require.Equal(t, replacementID, *replaced.ReplacedBy)
	require.Len(t, f.providers.sent, 1)
	require.Equal(t, replacementID, f.providers.sent[0].ID)
}

func TestSendText_Retry(t *testing.T) {
	t.Parallel()

	send := func(f *retryFixture) (*dto.SendTextResponse, error) {
		return f.svc.SendText(context.Background(), &dto.SendTextRequest{
			From: f.sender, To: f.recipient, DomainID: 1, Body: "again", RetryOf: &f.originalID,
		})
	}

	t.Run("replaces the failed original", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()

		resp, err := send(f)
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
		f.requireReplaced(t, resp.ID)
	})

	rejected := map[string]func(*model.RetryTarget){
		"not failed":      func(t *model.RetryTarget) { t.Failed = false },
		"other author":    func(t *model.RetryTarget) { t.AuthorID = uuid.New() },
		"already deleted": func(t *model.RetryTarget) { t.Deleted = true },
	}

	for name, mutate := range rejected {
		t.Run(name+" is rejected before any side effect", func(t *testing.T) {
			t.Parallel()

			f := newRetryFixture()
			mutate(f.target())

			_, err := send(f)
			require.Equal(t, codes.InvalidArgument, errors.Code(err))
			require.Zero(t, f.threader.calls, "no thread ensured")
			f.requireNoSideEffects(t)
		})
	}

	t.Run("a retry resolving to another thread is rejected before saving", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()
		f.target().ThreadID = uuid.New()

		_, err := send(f)
		require.Equal(t, codes.InvalidArgument, errors.Code(err))
		f.requireNoSideEffects(t)
	})

	t.Run("a re-sent retry returns the existing replacement", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()
		previous := uuid.New()
		f.markRetried(previous)

		resp, err := send(f)
		require.NoError(t, err)
		require.Equal(t, previous, resp.ID)
		require.Zero(t, f.threader.calls)
		f.requireNoSideEffects(t)
	})

	t.Run("a command-looking body is re-sent as text, not executed", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()
		botMemberID := uuid.New()
		f.thread.BotControllerID = &botMemberID
		bots := &fakeThreadManager{}
		f.svc.commands = NewCommandDispatcher([]Command{NewCloseCommand(bots, f.svc.logger)})

		resp, err := f.svc.SendText(context.Background(), &dto.SendTextRequest{
			From: f.sender, To: f.recipient, DomainID: 1, Body: model.CommandClose.String(), RetryOf: &f.originalID,
		})
		require.NoError(t, err)
		require.Zero(t, bots.releaseCalls, "/close must not run on retry")
		require.Nil(t, f.msgStore.lastSavedSystemMessage)
		f.requireReplaced(t, resp.ID)
	})

	t.Run("losing the race to an identical retry returns the winner", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()
		winner := uuid.New()
		f.msgStore.replaceFailedErr = errNotRetryable()
		f.msgStore.replaceHook = func() { f.markRetried(winner) }

		resp, err := send(f)
		require.NoError(t, err)
		require.Equal(t, winner, resp.ID)
		require.Empty(t, f.providers.sent, "the loser's message is rolled back, never sent")
	})
}

func TestSendDocument_Retry(t *testing.T) {
	t.Parallel()

	send := func(f *retryFixture) (*dto.SendDocumentResponse, error) {
		return f.svc.SendDocument(context.Background(), &dto.SendDocumentRequest{
			From: f.sender, To: f.recipient, DomainID: 1, RetryOf: &f.originalID,
			Document: dto.DocumentRequest{Documents: []*dto.Document{{ID: 42, Name: "report.pdf"}}},
		})
	}

	t.Run("replaces the failed original", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()

		resp, err := send(f)
		require.NoError(t, err)
		require.Equal(t, 1, f.media.processCalls)
		f.requireReplaced(t, resp.ID)
	})

	t.Run("a rejected retry processes no media and creates no thread", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()
		f.target().Failed = false

		_, err := send(f)
		require.Equal(t, codes.InvalidArgument, errors.Code(err))
		require.Zero(t, f.threader.calls)
		f.requireNoSideEffects(t)
	})

	t.Run("a re-sent retry returns the existing replacement", func(t *testing.T) {
		t.Parallel()

		f := newRetryFixture()
		previous := uuid.New()
		f.markRetried(previous)

		resp, err := send(f)
		require.NoError(t, err)
		require.Equal(t, previous, resp.ID)
		require.Zero(t, f.threader.calls)
		f.requireNoSideEffects(t)
	})
}

func TestClassifyProviderRPCError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantCode   string
		wantDetail string
	}{
		{"unavailable", status.Error(codes.Unavailable, "connection refused"), model.DeliveryErrorChannelUnavailable, "connection refused"},
		{"rate limited", status.Error(codes.ResourceExhausted, "too many requests"), model.DeliveryErrorChannelUnavailable, "too many requests"},
		{"unimplemented", status.Error(codes.Unimplemented, "location is not supported"), model.DeliveryErrorUnsupportedMedia, "location is not supported"},
		{"gate not found", status.Error(codes.NotFound, "gate not found"), model.DeliveryErrorSendFailed, "gate not found"},
		{"rejected", status.Error(codes.InvalidArgument, "bad recipient"), model.DeliveryErrorSendFailed, "bad recipient"},
		{"not a status", fmt.Errorf("boom"), model.DeliveryErrorSendFailed, "boom"},
		{
			"webitel error envelope is unwrapped",
			status.Error(codes.FailedPrecondition, `{"id":"api.process.internal","detail":"viber receiver not subscribed or unreachable","code":500,"status":"Internal Server Error"}`),
			model.DeliveryErrorSendFailed,
			"viber receiver not subscribed or unreachable",
		},
		{"envelope without detail is kept", status.Error(codes.Internal, `{"id":"x"}`), model.DeliveryErrorSendFailed, `{"id":"x"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, detail := classifyProviderRPCError(tc.err)
			require.Equal(t, tc.wantCode, code)
			require.Equal(t, tc.wantDetail, detail)
		})
	}
}

func TestProviderOutcomeUnknown(t *testing.T) {
	t.Parallel()

	unknown := []error{
		status.Error(codes.DeadlineExceeded, "context deadline exceeded"),
		status.Error(codes.Canceled, "context canceled"),
		context.DeadlineExceeded,
		context.Canceled,
	}
	for _, err := range unknown {
		require.True(t, providerOutcomeUnknown(err), "%v", err)
	}

	known := []error{
		status.Error(codes.Unavailable, "down"),
		status.Error(codes.FailedPrecondition, "rejected"),
		fmt.Errorf("boom"),
	}
	for _, err := range known {
		require.False(t, providerOutcomeUnknown(err), "%v", err)
	}
}

type recordingFailureRecorder struct {
	receipts []*model.StatusReceipt
	ctxErr   error
}

func (r *recordingFailureRecorder) MarkUnreportedFailed(ctx context.Context, receipts []*model.StatusReceipt) (int64, error) {
	r.ctxErr = ctx.Err()
	r.receipts = append(r.receipts, receipts...)

	return int64(len(receipts)), nil
}

func TestRecordSendFailure(t *testing.T) {
	t.Parallel()

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	newMessage := func(typ model.MessageType) *model.Message {
		return &model.Message{ID: uuid.New(), ThreadID: uuid.New(), DomainID: 7, Type: typ}
	}

	t.Run("records the peer's failure even when the request ctx is gone", func(t *testing.T) {
		t.Parallel()

		recorder := &recordingFailureRecorder{}
		adapter := &baseRPCProvidersAdapter{failures: recorder}

		msg := newMessage(model.MessageTypeText)
		peer := &model.ExternalPeerPair{ContactID: uuid.New(), Via: uuid.NewString()}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		adapter.recordSendFailure(ctx, msg, peer, status.Error(codes.Unavailable, "providers down"), discard)

		require.NoError(t, recorder.ctxErr)
		require.Len(t, recorder.receipts, 1)

		got := recorder.receipts[0]
		require.Equal(t, msg.ID, got.MessageID)
		require.Equal(t, msg.ThreadID, got.ThreadID)
		require.Equal(t, peer.ContactID, got.MemberID)
		require.Equal(t, int32(7), got.DomainID)
		require.Equal(t, "provider", got.Via)
		require.Equal(t, map[string]any{"code": model.DeliveryErrorChannelUnavailable, "message": "providers down"}, got.Error)
	})

	t.Run("every retryable kind is recorded", func(t *testing.T) {
		t.Parallel()

		for _, typ := range []model.MessageType{model.MessageTypeText, model.MessageTypeFile, model.MessageTypeLocation, model.MessageTypeContact} {
			recorder := &recordingFailureRecorder{}
			adapter := &baseRPCProvidersAdapter{failures: recorder}

			adapter.recordSendFailure(context.Background(), newMessage(typ), &model.ExternalPeerPair{ContactID: uuid.New()}, status.Error(codes.Unimplemented, "x"), discard)
			require.Len(t, recorder.receipts, 1, typ.String())
			require.Equal(t, model.DeliveryErrorUnsupportedMedia, recorder.receipts[0].Error["code"])
		}
	})

	skipped := []struct {
		name string
		msg  *model.Message
		err  error
	}{
		{"a message without ids", &model.Message{Type: model.MessageTypeText}, status.Error(codes.Unavailable, "x")},
		{"a system message", newMessage(model.MessageTypeSystem), status.Error(codes.Unavailable, "x")},
		{"an interactive message", newMessage(model.MessageTypeInteractive), status.Error(codes.Unavailable, "x")},
		{"a timed-out call", newMessage(model.MessageTypeText), status.Error(codes.DeadlineExceeded, "deadline")},
		{"a canceled call", newMessage(model.MessageTypeText), status.Error(codes.Canceled, "canceled")},
		{"a local deadline", newMessage(model.MessageTypeText), context.DeadlineExceeded},
	}

	for _, tc := range skipped {
		t.Run(tc.name+" is not recorded", func(t *testing.T) {
			t.Parallel()

			recorder := &recordingFailureRecorder{}
			adapter := &baseRPCProvidersAdapter{failures: recorder}

			adapter.recordSendFailure(context.Background(), tc.msg, &model.ExternalPeerPair{ContactID: uuid.New()}, tc.err, discard)
			require.Empty(t, recorder.receipts)
		})
	}
}

type failingProvidersAdapter struct{ *fakeProvidersAdapter }

func (failingProvidersAdapter) SendMessage(context.Context, *model.Message) error {
	return status.Error(codes.FailedPrecondition, "viber receiver not subscribed or unreachable")
}

func TestSendLocationAndContact_ChannelRejectionIsNotACallError(t *testing.T) {
	t.Parallel()

	newMsg := map[string]func(f *retryFixture) *model.Message{
		"location": func(f *retryFixture) *model.Message {
			return &model.Message{ID: uuid.New(), DomainID: 1, From: f.sender, SendTo: f.recipient, Type: model.MessageTypeLocation, Location: &model.MessageLocation{Latitude: 50.45, Longitude: 30.52}}
		},
		"contact": func(f *retryFixture) *model.Message {
			phone := "+380000000000"

			return &model.Message{ID: uuid.New(), DomainID: 1, From: f.sender, SendTo: f.recipient, Type: model.MessageTypeContact, Contact: &model.MessageContact{PhoneNumber: &phone}}
		},
	}

	send := func(f *retryFixture, kind string, msg *model.Message) (*model.Message, error) {
		if kind == "location" {
			return f.svc.SendLocation(context.Background(), msg)
		}

		return f.svc.SendContact(context.Background(), msg)
	}

	for kind, build := range newMsg {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			f := newRetryFixture()
			f.svc.providersAdapter = failingProvidersAdapter{&fakeProvidersAdapter{}}

			saved, err := send(f, kind, build(f))
			require.NoError(t, err)
			require.NotNil(t, saved)
			require.Equal(t, f.thread.ID, saved.ThreadID)
			require.Zero(t, f.msgStore.replaceFailedCalls)
		})

		t.Run(kind+" retry replaces the original", func(t *testing.T) {
			t.Parallel()

			f := newRetryFixture()
			msg := build(f)
			msg.RetryOf = &f.originalID

			saved, err := send(f, kind, msg)
			require.NoError(t, err)
			f.requireReplaced(t, saved.ID)
		})

		t.Run(kind+" rejected retry has no side effects", func(t *testing.T) {
			t.Parallel()

			f := newRetryFixture()
			f.target().AuthorID = uuid.New()
			msg := build(f)
			msg.RetryOf = &f.originalID

			_, err := send(f, kind, msg)
			require.Equal(t, codes.InvalidArgument, errors.Code(err))
			require.Zero(t, f.threader.calls)
			f.requireNoSideEffects(t)
		})

		t.Run(kind+" re-sent retry returns the existing replacement", func(t *testing.T) {
			t.Parallel()

			f := newRetryFixture()
			previous := uuid.New()
			f.markRetried(previous)
			msg := build(f)
			msg.RetryOf = &f.originalID

			saved, err := send(f, kind, msg)
			require.NoError(t, err)
			require.Equal(t, previous, saved.ID)
			require.Equal(t, f.thread.ID, saved.ThreadID)
			require.Zero(t, f.threader.calls)
			f.requireNoSideEffects(t)
		})
	}
}
