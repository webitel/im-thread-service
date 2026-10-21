package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	contactv1 "github.com/webitel/im-thread-service/gen/go/contact/v1"
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type fakeContactDirectory struct{}

func (fakeContactDirectory) GetIdentity(context.Context, uuid.UUID, int) (*imcontact.Identity, error) {
	return nil, nil //nolint:nilnil // unknown contact: the event just lacks enrichment
}

func (fakeContactDirectory) SearchContact(context.Context, *contactv1.SearchContactRequest) (*contactv1.ContactList, error) {
	return &contactv1.ContactList{}, nil
}

func newDeleteTestService(msgStore *fakeMessageStore, statuses *recordingMessageStatusStore) *MessageService {
	return &MessageService{
		tagHooks:      &fakeTagHooks{},
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		contactClient: fakeContactDirectory{},
		uow: fakeUnitOfWork{
			messageStore:       msgStore,
			outboxStore:        &fakeOutboxStore{},
			threadDialogStore:  &fakeThreadDialogStore{},
			messageStatusStore: statuses,
		},
	}
}

func TestDeleteMessages_RecountsUnread(t *testing.T) {
	t.Parallel()

	deleter := shared.Peer{ID: uuid.New(), Type: shared.PeerContact}

	t.Run("every thread with a deleted message is recounted once, sorted", func(t *testing.T) {
		t.Parallel()

		threadA, threadB := uuid.New(), uuid.New()
		deleted := []*model.Message{
			{ID: uuid.New(), ThreadID: threadB},
			{ID: uuid.New(), ThreadID: threadA},
			{ID: uuid.New(), ThreadID: threadB},
		}

		msgStore := &fakeMessageStore{deleteMessagesResult: &model.MessageDeleteResult{Deleted: deleted}}
		statuses := &recordingMessageStatusStore{}

		out, err := newDeleteTestService(msgStore, statuses).DeleteMessages(context.Background(), &dto.DeleteMessagesRequest{
			DeletedBy: deleter, IDs: uuid.UUIDs{deleted[0].ID, deleted[1].ID, deleted[2].ID}, DomainID: 1,
		})
		require.NoError(t, err)
		require.Len(t, out.DeletedIDs, 3)

		want := []uuid.UUID{threadA, threadB}
		slices.SortFunc(want, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
		require.Equal(t, want, statuses.recountedThreads)
	})

	t.Run("a retry that deletes nothing recounts nothing", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		msgStore := &fakeMessageStore{deleteMessagesResult: &model.MessageDeleteResult{
			Skipped: []model.MessageSkip{{ID: id, Reason: model.MessageSkipAlreadyDeleted}},
		}}
		statuses := &recordingMessageStatusStore{}

		_, err := newDeleteTestService(msgStore, statuses).DeleteMessages(context.Background(), &dto.DeleteMessagesRequest{
			DeletedBy: deleter, IDs: uuid.UUIDs{id}, DomainID: 1,
		})
		require.NoError(t, err)
		require.Empty(t, statuses.recountedThreads)
	})
}
