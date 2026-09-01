package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// Embeds noopThreadTagStore so it only needs to implement ListForContact.
type stubTagStore struct {
	noopThreadTagStore

	tagsByThread map[uuid.UUID][]*model.ThreadTag
	listErr      error

	gotContactID uuid.UUID
	gotThreadIDs []uuid.UUID
	listCalls    int
}

func (s *stubTagStore) ListForContact(ctx context.Context, contactID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID][]*model.ThreadTag, error) {
	s.listCalls++
	s.gotContactID = contactID
	s.gotThreadIDs = threadIDs

	return s.tagsByThread, s.listErr
}

func newTagService(stub *stubTagStore) *ThreadManagementService {
	return NewThreadService(nil, fakeUnitOfWork{threadTagStore: stub}, nil, nil)
}

func TestEnrichTags_SetsPerThreadTags(t *testing.T) {
	t1, t2, t3 := uuid.New(), uuid.New(), uuid.New()
	tag1 := &model.ThreadTag{ID: uuid.New(), Tag: "important"}
	tag2 := &model.ThreadTag{ID: uuid.New(), Tag: "urgent"}

	stub := &stubTagStore{
		tagsByThread: map[uuid.UUID][]*model.ThreadTag{
			t1: {tag1},
			t3: {tag2},
		},
	}
	svc := newTagService(stub)

	threads := []*model.Thread{
		{ID: t1, TagLookupID: t1},
		{ID: t2, TagLookupID: t2},
		{ID: t3, TagLookupID: t3},
	}

	callerID := uuid.New()
	svc.enrichTags(context.Background(), callerID, threads)

	require.Equal(t, []*model.ThreadTag{tag1}, threads[0].Tags)
	require.Nil(t, threads[1].Tags, "threads with no tags stay nil")
	require.Equal(t, []*model.ThreadTag{tag2}, threads[2].Tags)

	require.Equal(t, callerID, stub.gotContactID)
	require.Equal(t, []uuid.UUID{t1, t2, t3}, stub.gotThreadIDs)
}

// Guards against keying enrichment off Thread.ID again, which for
// SearchLeft results is a membership-period id, not the thread id.
func TestEnrichTags_KeysByTagLookupIDNotID(t *testing.T) {
	dialogID, threadID := uuid.New(), uuid.New()
	tag := &model.ThreadTag{ID: uuid.New(), Tag: "left-thread-tag"}

	stub := &stubTagStore{
		tagsByThread: map[uuid.UUID][]*model.ThreadTag{threadID: {tag}},
	}
	svc := newTagService(stub)

	threads := []*model.Thread{{ID: dialogID, TagLookupID: threadID}}

	svc.enrichTags(context.Background(), uuid.New(), threads)

	require.Equal(t, []uuid.UUID{threadID}, stub.gotThreadIDs, "must query by TagLookupID, not ID")
	require.Equal(t, []*model.ThreadTag{tag}, threads[0].Tags)
}

func TestEnrichTags_NoCallerIDSkipsQuery(t *testing.T) {
	stub := &stubTagStore{tagsByThread: map[uuid.UUID][]*model.ThreadTag{uuid.New(): {}}}
	svc := newTagService(stub)

	id := uuid.New()
	threads := []*model.Thread{{ID: id, TagLookupID: id}}

	svc.enrichTags(context.Background(), uuid.Nil, threads)

	require.Zero(t, stub.listCalls, "no caller id must not hit the store")
	require.Nil(t, threads[0].Tags)
}

func TestEnrichTags_ErrorLeavesEmptyTags(t *testing.T) {
	stub := &stubTagStore{listErr: errors.New("boom")}
	svc := newTagService(stub)

	id := uuid.New()
	threads := []*model.Thread{{ID: id, TagLookupID: id}}

	svc.enrichTags(context.Background(), uuid.New(), threads)

	require.Nil(t, threads[0].Tags, "a tags failure must not corrupt the thread")
}
