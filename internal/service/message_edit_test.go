package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func newEditTestService(msgStore *fakeMessageStore, outbox *fakeOutboxStore) *MessageService {
	return &MessageService{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow: fakeUnitOfWork{
			messageStore:      msgStore,
			outboxStore:       outbox,
			threadDialogStore: &fakeThreadDialogStore{},
		},
	}
}

func TestEditMessage(t *testing.T) {
	t.Parallel()

	editorID := uuid.New()
	threadID := uuid.New()
	msgID := uuid.New()

	newMsg := func() *model.Message {
		return &model.Message{
			ID:       msgID,
			ThreadID: threadID,
			From:     shared.Peer{ID: editorID, Type: shared.PeerContact},
			Body:     "corrected text",
		}
	}

	t.Run("allowed edit emits an edited event carrying the editor and recipients", func(t *testing.T) {
		t.Parallel()

		recipientID := uuid.New()
		msgStore := &fakeMessageStore{}
		outbox := &fakeOutboxStore{}
		dialogs := &fakeThreadDialogStore{quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: editorID},
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: recipientID},
		}}

		svc := &MessageService{
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			uow: fakeUnitOfWork{
				messageStore:      msgStore,
				outboxStore:       outbox,
				threadDialogStore: dialogs,
			},
		}

		out, err := svc.EditMessage(context.Background(), newMsg())
		require.NoError(t, err)
		require.Equal(t, msgID, out.ID)

		// The editor identity is forwarded to the store (which authorizes in-query).
		require.Equal(t, 1, msgStore.editMessageCalls)
		require.Equal(t, editorID, msgStore.lastEditedMessage.From.ID)

		require.Len(t, outbox.published, 1)
		require.True(t, strings.Contains(outbox.published[0].topic, ".message.edited.v1"),
			"edit must route on the edited topic, got %q", outbox.published[0].topic)

		edited, ok := outbox.published[0].event.(*event.MessageEdited)
		require.True(t, ok, "published event must be *event.MessageEdited")
		require.Equal(t, editorID, edited.EditedBy.ContactID)

		// The recipient list (loaded from the thread) must ride on the event so
		// delivery can fan the edit out to the same participants.
		contactIDs := make([]uuid.UUID, 0, len(edited.To))
		for _, m := range edited.To {
			contactIDs = append(contactIDs, m.ContactID)
		}

		require.ElementsMatch(t, []uuid.UUID{editorID, recipientID}, contactIDs)
	})

	t.Run("store rejection (not author / closed chat / missing) is propagated and emits nothing", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{
			editMessageErr: errors.Forbidden("nope", errors.WithID("postgres.message.edit.not_allowed")),
		}
		outbox := &fakeOutboxStore{}
		svc := newEditTestService(msgStore, outbox)

		_, err := svc.EditMessage(context.Background(), newMsg())
		require.Error(t, err)
		require.Equal(t, "postgres.message.edit.not_allowed", errors.ID(err))
		require.Empty(t, outbox.published, "no event on a rejected edit")
	})

	t.Run("missing message id is rejected before touching the store", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{}
		svc := newEditTestService(msgStore, &fakeOutboxStore{})

		msg := newMsg()
		msg.ID = uuid.Nil

		_, err := svc.EditMessage(context.Background(), msg)
		require.Error(t, err)
		require.Equal(t, 0, msgStore.editMessageCalls)
	})

	t.Run("missing editor identity is rejected before touching the store", func(t *testing.T) {
		t.Parallel()

		msgStore := &fakeMessageStore{}
		svc := newEditTestService(msgStore, &fakeOutboxStore{})

		msg := newMsg()
		msg.From = shared.Peer{}

		_, err := svc.EditMessage(context.Background(), msg)
		require.Error(t, err)
		require.Equal(t, 0, msgStore.editMessageCalls)
	})
}
