package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

type fakeMessageReactionStore struct {
	result       *model.ReactionResult
	err          error
	calls        int
	lastReaction *model.Reaction
	aggregate    []*model.MessageReaction
}

func (f *fakeMessageReactionStore) SetReaction(_ context.Context, r *model.Reaction) (*model.ReactionResult, error) {
	f.calls++
	f.lastReaction = r

	if f.err != nil {
		return nil, f.err
	}

	return f.result, nil
}

func (f *fakeMessageReactionStore) AggregateForMessage(_ context.Context, _ uuid.UUID) ([]*model.MessageReaction, error) {
	return f.aggregate, nil
}

type fakeReactionProvidersAdapter struct {
	sendReactionCalls int
	lastResult        *model.ReactionResult
}

func (f *fakeReactionProvidersAdapter) SendMessage(context.Context, *model.Message) error {
	return nil
}

func (f *fakeReactionProvidersAdapter) SendTyping(context.Context, *model.TypingDispatch) error {
	return nil
}

func (f *fakeReactionProvidersAdapter) SendReaction(_ context.Context, _ *model.Reaction, res *model.ReactionResult) error {
	f.sendReactionCalls++
	f.lastResult = res

	return nil
}

func newReactionTestService(
	reactions *fakeMessageReactionStore,
	dialogs *fakeThreadDialogStore,
	outbox *fakeOutboxStore,
	providers *fakeReactionProvidersAdapter,
) *MessageService {
	return &MessageService{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow: fakeUnitOfWork{
			messageReactionStore: reactions,
			threadDialogStore:    dialogs,
			outboxStore:          outbox,
		},
		providersAdapter: providers,
	}
}

func TestSetReaction(t *testing.T) {
	t.Parallel()

	reactorID := uuid.New()
	threadID := uuid.New()
	msgID := uuid.New()

	newRequest := func() *dto.SetReactionRequest {
		return &dto.SetReactionRequest{
			Reactor:        shared.Peer{ID: reactorID, Type: shared.PeerContact},
			MessageID:      msgID,
			Emoji:          "👍",
			DomainID:       1,
			IdempotencyKey: "send-1",
		}
	}

	t.Run("a changed reaction emits one reaction event and is forwarded", func(t *testing.T) {
		t.Parallel()

		recipientID := uuid.New()
		reactions := &fakeMessageReactionStore{result: &model.ReactionResult{
			Action:    model.ReactionActionSet,
			Emoji:     "👍",
			ThreadID:  threadID,
			ReactedAt: time.UnixMilli(1000).UTC(),
			Changed:   true,
		}}
		dialogs := &fakeThreadDialogStore{quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: reactorID},
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: recipientID},
		}}
		outbox := &fakeOutboxStore{}
		providers := &fakeReactionProvidersAdapter{}

		svc := newReactionTestService(reactions, dialogs, outbox, providers)

		out, err := svc.SetReaction(context.Background(), newRequest())
		require.NoError(t, err)
		require.Equal(t, model.ReactionActionSet, out.Action)
		require.Equal(t, "👍", out.Emoji)

		// The store receives the reactor and idempotency key untouched.
		require.Equal(t, 1, reactions.calls)
		require.Equal(t, reactorID, reactions.lastReaction.ReactorID)
		require.Equal(t, "send-1", reactions.lastReaction.IdempotencyKey)

		require.Len(t, outbox.published, 1)
		require.True(t, strings.Contains(outbox.published[0].topic, ".message.reaction.v1"),
			"reaction must route on the reaction topic, got %q", outbox.published[0].topic)

		reaction, ok := outbox.published[0].event.(*event.MessageReaction)
		require.True(t, ok, "published event must be *event.MessageReaction")
		require.Equal(t, event.ReactionActionSet, reaction.Action)
		require.Equal(t, "👍", reaction.Emoji)
		require.Equal(t, reactorID, reaction.Reactor.ContactID)

		// The recipient list rides on the event so delivery fans out to both sides.
		contactIDs := make([]uuid.UUID, 0, len(reaction.To))
		for _, m := range reaction.To {
			contactIDs = append(contactIDs, m.ContactID)
		}

		require.ElementsMatch(t, []uuid.UUID{reactorID, recipientID}, contactIDs)

		// The change is forwarded best-effort to external providers.
		require.Equal(t, 1, providers.sendReactionCalls)
	})

	t.Run("an unchanged reaction publishes nothing and is not forwarded", func(t *testing.T) {
		t.Parallel()

		reactions := &fakeMessageReactionStore{result: &model.ReactionResult{
			Action:   model.ReactionActionUnchanged,
			ThreadID: threadID,
			Changed:  false,
		}}
		outbox := &fakeOutboxStore{}
		providers := &fakeReactionProvidersAdapter{}

		svc := newReactionTestService(reactions, &fakeThreadDialogStore{}, outbox, providers)

		out, err := svc.SetReaction(context.Background(), newRequest())
		require.NoError(t, err)
		require.Equal(t, model.ReactionActionUnchanged, out.Action)
		require.Empty(t, outbox.published, "no event on an unchanged reaction")
		require.Zero(t, providers.sendReactionCalls, "no forward on an unchanged reaction")
	})

	t.Run("a store rejection is surfaced as forbidden and emits nothing", func(t *testing.T) {
		t.Parallel()

		reactions := &fakeMessageReactionStore{err: store.ErrReactionNotAllowed}
		outbox := &fakeOutboxStore{}
		providers := &fakeReactionProvidersAdapter{}

		svc := newReactionTestService(reactions, &fakeThreadDialogStore{}, outbox, providers)

		_, err := svc.SetReaction(context.Background(), newRequest())
		require.Error(t, err)
		require.Equal(t, "service.message.set_reaction.not_allowed", errors.ID(err))
		require.Empty(t, outbox.published)
		require.Zero(t, providers.sendReactionCalls)
	})

	t.Run("missing message id is rejected before touching the store", func(t *testing.T) {
		t.Parallel()

		reactions := &fakeMessageReactionStore{}
		svc := newReactionTestService(reactions, &fakeThreadDialogStore{}, &fakeOutboxStore{}, &fakeReactionProvidersAdapter{})

		req := newRequest()
		req.MessageID = uuid.Nil

		_, err := svc.SetReaction(context.Background(), req)
		require.Error(t, err)
		require.Zero(t, reactions.calls)
	})
}
