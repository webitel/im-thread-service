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
	states     map[uuid.UUID][]model.MemberReadState
	statesErr  error

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

func (s *stubStatusStore) ReadMemberStates(_ context.Context, domainID int32, memberID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID][]model.MemberReadState, error) {
	s.gotDomainID = domainID
	s.gotMemberID = memberID
	s.gotThreadIDs = threadIDs

	return s.states, s.statesErr
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

func TestEnrichReadStates_SetsPerMemberStates(t *testing.T) {
	t1, t2, t3 := uuid.New(), uuid.New(), uuid.New()
	m1, m2 := uuid.New(), uuid.New()
	stub := &stubStatusStore{states: map[uuid.UUID][]model.MemberReadState{
		t1: {
			{ThreadID: t1, MemberID: m1, DeliveredUpToSeq: 10, ReadUpToSeq: 8},
			{ThreadID: t1, MemberID: m2, DeliveredUpToSeq: 5, ReadUpToSeq: 5},
		},
		t3: {{ThreadID: t3, MemberID: m1, ReadUpToSeq: 2}},
	}}
	svc := newUnreadService(stub)

	threads := []*model.Thread{{ID: t1}, {ID: t2}, {ID: t3}}

	svc.enrichReadStates(context.Background(), uuid.New(), 7, threads)

	require.Len(t, threads[0].ReadStates, 2)
	require.Equal(t, int64(10), threads[0].ReadStates[0].DeliveredUpToSeq)
	require.Equal(t, int64(8), threads[0].ReadStates[0].ReadUpToSeq)

	require.Empty(t, threads[1].ReadStates, "threads with no rows stay empty")

	require.Len(t, threads[2].ReadStates, 1)
	require.Equal(t, int64(2), threads[2].ReadStates[0].ReadUpToSeq)

	require.Equal(t, int32(7), stub.gotDomainID)
	require.Equal(t, []uuid.UUID{t1, t2, t3}, stub.gotThreadIDs)
}

func TestEnrichReadStates_NoSelfIDSkipsQuery(t *testing.T) {
	tid := uuid.New()
	stub := &stubStatusStore{states: map[uuid.UUID][]model.MemberReadState{
		tid: {{ThreadID: tid, MemberID: uuid.New(), ReadUpToSeq: 9}},
	}}
	svc := newUnreadService(stub)

	threads := []*model.Thread{{ID: uuid.New()}}

	svc.enrichReadStates(context.Background(), uuid.Nil, 0, threads)

	require.Empty(t, threads[0].ReadStates)
	require.Nil(t, stub.gotThreadIDs, "no self id must not hit the store")
}

func TestEnrichReadStates_ErrorLeavesEmpty(t *testing.T) {
	stub := &stubStatusStore{statesErr: errors.New("boom")}
	svc := newUnreadService(stub)

	threads := []*model.Thread{{ID: uuid.New()}}

	svc.enrichReadStates(context.Background(), uuid.New(), 0, threads)

	require.Empty(t, threads[0].ReadStates, "a read-state failure must not corrupt the thread")
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
