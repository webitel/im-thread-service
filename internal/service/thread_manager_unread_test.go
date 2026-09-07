package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// stubStatusStore is a configurable MessageStatusStore for unread tests.
// It embeds noopMessageStatusStore to satisfy the rest of the interface.
type stubStatusStore struct {
	noopMessageStatusStore

	counts     map[uuid.UUID]int64
	countErr   error
	summary    model.UnreadSummary
	summaryErr error

	gotDomainID  int32
	gotMemberID  uuid.UUID
	gotThreadIDs []uuid.UUID
	countCalls   int
}

func (s *stubStatusStore) ReadUnread(_ context.Context, domainID int32, memberID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	s.countCalls++
	s.gotDomainID = domainID
	s.gotMemberID = memberID
	s.gotThreadIDs = threadIDs

	return s.counts, s.countErr
}

func (s *stubStatusStore) UnreadSummary(_ context.Context, _ int32, _ uuid.UUID) (model.UnreadSummary, error) {
	return s.summary, s.summaryErr
}

func newUnreadService(stub *stubStatusStore) *ThreadManagementService {
	return NewThreadService(nil, fakeUnitOfWork{messageStatusStore: stub}, nil, nil, nil)
}

func TestEnrichUnread_SetsPerThreadCount(t *testing.T) {
	t1, t2, t3 := uuid.New(), uuid.New(), uuid.New()
	stub := &stubStatusStore{counts: map[uuid.UUID]int64{t1: 5, t3: 2}}
	svc := newUnreadService(stub)

	threads := []*model.Thread{{ID: t1}, {ID: t2}, {ID: t3}}

	svc.enrichUnread(context.Background(), uuid.New(), 7, threads)

	require.Equal(t, int64(5), threads[0].UnreadCount)
	require.Equal(t, int64(0), threads[1].UnreadCount, "threads with no unread rows stay zero")
	require.Equal(t, int64(2), threads[2].UnreadCount)

	require.Equal(t, int32(7), stub.gotDomainID)
	require.Equal(t, []uuid.UUID{t1, t2, t3}, stub.gotThreadIDs)
}

func TestEnrichUnread_NoSelfIDSkipsQuery(t *testing.T) {
	stub := &stubStatusStore{counts: map[uuid.UUID]int64{uuid.New(): 9}}
	svc := newUnreadService(stub)

	threads := []*model.Thread{{ID: uuid.New()}}

	svc.enrichUnread(context.Background(), uuid.Nil, 0, threads)

	require.Zero(t, stub.countCalls, "no self id must not hit the store")
	require.Equal(t, int64(0), threads[0].UnreadCount)
}

func TestEnrichUnread_ErrorLeavesZeroCounts(t *testing.T) {
	stub := &stubStatusStore{countErr: errors.New("boom")}
	svc := newUnreadService(stub)

	threads := []*model.Thread{{ID: uuid.New()}}

	svc.enrichUnread(context.Background(), uuid.New(), 0, threads)

	require.Equal(t, int64(0), threads[0].UnreadCount, "a count failure must not corrupt the thread")
}

func TestGetUnreadSummary_RequiresSelfID(t *testing.T) {
	svc := newUnreadService(&stubStatusStore{})

	_, err := svc.GetUnreadSummary(context.Background(), &dto.UnreadSummaryRequest{})

	require.Error(t, err)
}

func TestGetUnreadSummary_ReturnsStoreResult(t *testing.T) {
	stub := &stubStatusStore{summary: model.UnreadSummary{Chats: 3, Messages: 12}}
	svc := newUnreadService(stub)

	got, err := svc.GetUnreadSummary(context.Background(), &dto.UnreadSummaryRequest{
		SelfID:   uuid.New(),
		DomainID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, int32(3), got.Chats)
	require.Equal(t, int64(12), got.Messages)
}
