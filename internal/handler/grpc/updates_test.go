package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/adapter/journal"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type fakeUpdates struct {
	cursor    string
	trimmed   int64
	changes   *journal.ContactChanges
	events    map[string][]journal.Event
	notMember map[string]bool
	unread    int64
	members   []journal.Member
}

func (f *fakeUpdates) ContactChanges(context.Context, string, string) (*journal.ContactChanges, error) {
	return f.changes, nil
}

func (f *fakeUpdates) ChangesSince(_ context.Context, threadID string, _ *journal.ContactChanges, limit int) ([]journal.Event, error) {
	ev := f.events[threadID]

	return ev[:min(len(ev), limit+1)], nil
}

func (f *fakeUpdates) ContactCursor(context.Context, string) (string, error) { return f.cursor, nil }

func (f *fakeUpdates) TrimHorizon(context.Context) (int64, error) { return f.trimmed, nil }

func (f *fakeUpdates) ReadStates(context.Context, string) ([]journal.ReadState, error) {
	return nil, nil
}

func (f *fakeUpdates) IsMember(_ context.Context, threadID, _ string, _ int32) (bool, error) {
	return !f.notMember[threadID], nil
}

func (f *fakeUpdates) Members(context.Context, string) ([]journal.Member, error) {
	return f.members, nil
}

func (f *fakeUpdates) Unread(context.Context, string, string) (int64, error) { return f.unread, nil }

// fakeHistory returns stored messages by id, or the newest one when no ids are asked.
type fakeHistory struct {
	MessageHistoryService

	byID   map[uuid.UUID]*model.Message
	newest *model.Message
}

func (f *fakeHistory) Search(_ context.Context, in *dto.HistoryMessageInputDTO) (model.MessageSlice, queryobject.PageInfo[queryobject.MessageHistoryCursor], error) {
	if len(in.IDs) == 0 && f.newest != nil {
		return model.MessageSlice{f.newest}, queryobject.PageInfo[queryobject.MessageHistoryCursor]{}, nil
	}

	out := make(model.MessageSlice, 0, len(in.IDs))
	for _, id := range in.IDs {
		if m, ok := f.byID[id]; ok {
			out = append(out, m)
		}
	}

	return out, queryobject.PageInfo[queryobject.MessageHistoryCursor]{}, nil
}

type fakeThreads struct {
	ThreadManagementService

	byID map[uuid.UUID]*model.Thread
}

func (f *fakeThreads) Search(_ context.Context, req *dto.ThreadSearchRequest) ([]*model.Thread, error) {
	out := make([]*model.Thread, 0, len(req.IDs))
	for _, id := range req.IDs {
		if t, ok := f.byID[id]; ok {
			out = append(out, t)
		}
	}

	return out, nil
}

func updatesReq(cursor string) *impb.GetUpdatesRequest {
	return &impb.GetUpdatesRequest{CallerId: uuid.NewString(), DomainId: 1, Cursor: cursor}
}

func msgEvent(kind string, msgID uuid.UUID) journal.Event {
	return journal.Event{Kind: kind, Fields: map[string]string{journal.FieldMsgID: msgID.String()}}
}

func TestGetUpdates_Errors(t *testing.T) {
	srv := NewUpdatesServer(&fakeHistory{}, &fakeUpdates{}, &fakeThreads{})

	for name, req := range map[string]*impb.GetUpdatesRequest{
		"malformed caller id": {CallerId: "x", DomainId: 1},
		"missing domain":      {CallerId: uuid.NewString()},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := srv.GetUpdates(context.Background(), req)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, errors.Code(err))
		})
	}
}

// First sync: the client gets its starting cursor and loads the thread list itself.
func TestGetUpdates_FirstSyncResyncs(t *testing.T) {
	resp, err := NewUpdatesServer(&fakeHistory{}, &fakeUpdates{cursor: "77"}, &fakeThreads{}).
		GetUpdates(context.Background(), updatesReq(""))
	require.NoError(t, err)
	assert.True(t, resp.GetResync())
	assert.Equal(t, "77", resp.GetCursor())
}

// Every changed thread comes back: an existing one as a folded diff, a thread created since
// the cursor with its dialog and top message, a left one flagged.
func TestGetUpdates_AllChangedThreads(t *testing.T) {
	oldThread, newThread, leftThread := uuid.New(), uuid.New(), uuid.New()
	edited, created := uuid.New(), uuid.New()
	deletedAt := time.Now()
	gone := uuid.New()

	updates := &fakeUpdates{
		changes: &journal.ContactChanges{Cursor: "900", After: 100, Horizon: 900, Threads: []string{
			oldThread.String(), newThread.String(), leftThread.String(),
		}},
		events: map[string][]journal.Event{
			oldThread.String(): {
				msgEvent(journal.KindMessageEdited, edited),
				msgEvent(journal.KindMessageReaction, edited),
				msgEvent(journal.KindMessageDeleted, gone),
			},
			newThread.String(): {{Kind: journal.KindThreadCreated}, msgEvent(journal.KindMessageNew, created)},
		},
		notMember: map[string]bool{leftThread.String(): true},
		unread:    2,
	}
	history := &fakeHistory{
		byID: map[uuid.UUID]*model.Message{
			edited:  {ID: edited, Body: "final", Seq: 4, RevisionCount: 1, UpdatedAt: time.UnixMilli(9)},
			gone:    {ID: gone, DeletedAt: &deletedAt},
			created: {ID: created, Body: "hello", Seq: 1},
		},
		newest: &model.Message{ID: created, Body: "hello", Seq: 1},
	}
	threads := &fakeThreads{byID: map[uuid.UUID]*model.Thread{newThread: {
		ID: newThread, Subject: "new dialog", LastMessage: &model.Message{ID: created, Body: "hello"},
	}}}

	resp, err := NewUpdatesServer(history, updates, threads).GetUpdates(context.Background(), updatesReq("100"))
	require.NoError(t, err)
	assert.Equal(t, "900", resp.GetCursor())
	require.Len(t, resp.GetThreads(), 3)

	byID := map[string]*impb.ThreadUpdates{}
	for _, e := range resp.GetThreads() {
		byID[e.GetThreadId()] = e
	}

	old := byID[oldThread.String()]
	require.Len(t, old.GetMessages(), 1, "edit and reaction fold into one message")
	assert.Equal(t, "final", old.GetMessages()[0].GetBody())
	assert.Equal(t, []string{gone.String()}, old.GetDeletedMessageIds())
	assert.Nil(t, old.GetDialog(), "the caller already knows this thread")
	assert.Equal(t, int64(2), old.GetUnreadCount())

	fresh := byID[newThread.String()]
	assert.Equal(t, "new dialog", fresh.GetDialog().GetSubject())
	assert.Nil(t, fresh.GetDialog().GetLastMsg(), "the preview is top_message only")
	assert.Equal(t, "hello", fresh.GetTopMessage().GetBody())
	require.Len(t, fresh.GetMessages(), 1)

	assert.True(t, byID[leftThread.String()].GetLeft())
}

// A thread is new to the caller when it was created or they joined it inside the window.
func TestIsNewToCaller(t *testing.T) {
	me := uuid.NewString()
	joined := journal.Event{Kind: journal.KindMemberChanged, Fields: map[string]string{
		journal.FieldContactID: me, journal.FieldAction: journal.ActionJoined,
	}}
	otherJoined := journal.Event{Kind: journal.KindMemberChanged, Fields: map[string]string{
		journal.FieldContactID: uuid.NewString(), journal.FieldAction: journal.ActionJoined,
	}}

	assert.True(t, isNewToCaller([]journal.Event{joined}, me))
	assert.False(t, isNewToCaller([]journal.Event{otherJoined}, me))
	assert.True(t, isNewToCaller([]journal.Event{{Kind: journal.KindThreadCreated}}, me))
	assert.False(t, isNewToCaller([]journal.Event{msgEvent(journal.KindMessageNew, uuid.New())}, me))
}

func TestGetUpdates_Resyncs(t *testing.T) {
	thread := uuid.NewString()
	many := make([]journal.Event, journal.MaxContactChanges+1)

	for i := range many {
		many[i] = msgEvent(journal.KindMessageNew, uuid.New())
	}

	tests := map[string]*fakeUpdates{
		"more than 1000 threads": {cursor: "5", changes: &journal.ContactChanges{TooMany: true}},
		"cursor older than retention": {cursor: "5", trimmed: 200, changes: &journal.ContactChanges{
			After: 100, Threads: []string{thread},
		}},
		"more than 1000 changes": {cursor: "5", changes: &journal.ContactChanges{
			After: 100, Threads: []string{thread},
		}, events: map[string][]journal.Event{thread: many}},
	}

	for name, updates := range tests {
		t.Run(name, func(t *testing.T) {
			resp, err := NewUpdatesServer(&fakeHistory{}, updates, &fakeThreads{}).GetUpdates(context.Background(), updatesReq("100"))
			require.NoError(t, err)
			assert.True(t, resp.GetResync())
			assert.Equal(t, "5", resp.GetCursor())
			assert.Empty(t, resp.GetThreads())
		})
	}
}

// fakeCursors hands out one cursor per contact, the way the journal does.
type fakeCursors map[string]string

func (f fakeCursors) ContactCursor(_ context.Context, contactID string) (string, error) {
	return f[contactID], nil
}

// History hands out the GetUpdates cursor read before the page, so the client needs no second counter.
func TestSearchThreadMessagesHistory_ReturnsUpdatesCursor(t *testing.T) {
	caller := uuid.NewString()
	srv := NewMessageHistoryServer(&fakeHistory{}, fakeCursors{caller: "92547098"})

	resp, err := srv.SearchThreadMessagesHistory(context.Background(), &impb.SearchMessageHistoryRequest{
		ThreadId: uuid.NewString(), CallerId: caller, DomainId: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "92547098", resp.GetUpdatesCursor())
}

// Search hands out the GetUpdates cursor on the response, so the thread list and the catch-up agree.
func TestThreadSearch_ReturnsUpdatesCursor(t *testing.T) {
	caller := uuid.NewString()
	srv := NewThreadService(&fakeThreads{}, nil, nil, fakeCursors{caller: "92547098"})

	resp, err := srv.Search(context.Background(), &impb.ThreadSearchRequest{SelfId: caller, DomainIds: []int32{1}, Size: 10})
	require.NoError(t, err)
	assert.Equal(t, "92547098", resp.GetUpdatesCursor(), "the same cursor history hands out")
}

// An old cursor for a contact with no changes since stays valid: nothing was trimmed from them.
func TestGetUpdates_TrimmedWithoutChangesKeepsCursor(t *testing.T) {
	updates := &fakeUpdates{cursor: "100", trimmed: 500, changes: &journal.ContactChanges{Cursor: "100", After: 100}}

	resp, err := NewUpdatesServer(&fakeHistory{}, updates, &fakeThreads{}).GetUpdates(context.Background(), updatesReq("100"))
	require.NoError(t, err)
	assert.False(t, resp.GetResync())
	assert.Equal(t, "100", resp.GetCursor())
	assert.Empty(t, resp.GetThreads())
}

func TestUpdatesCursor_Get(t *testing.T) {
	caller := uuid.NewString()
	srv := NewUpdatesCursorServer(fakeCursors{caller: "94770179"})

	resp, err := srv.Get(context.Background(), &impb.GetUpdatesCursorRequest{CallerId: caller})
	require.NoError(t, err)
	assert.Equal(t, "94770179", resp.GetCursor())

	_, err = srv.Get(context.Background(), &impb.GetUpdatesCursorRequest{CallerId: "x"})
	assert.Equal(t, codes.InvalidArgument, errors.Code(err))
}

// A read on another device changes only the thread's horizons: it arrives with the fresh
// unread count and no messages.
func TestGetUpdates_ReadOnlyChange(t *testing.T) {
	thread := uuid.NewString()
	updates := &fakeUpdates{
		changes: &journal.ContactChanges{Cursor: "300", After: 200, Threads: []string{thread}},
		events: map[string][]journal.Event{thread: {{Kind: journal.KindReadChanged, Fields: map[string]string{
			journal.FieldActor: uuid.NewString(), journal.FieldUpToSeq: "9",
		}}}},
		unread: 0,
	}

	resp, err := NewUpdatesServer(&fakeHistory{}, updates, &fakeThreads{}).GetUpdates(context.Background(), updatesReq("200"))
	require.NoError(t, err)
	require.Len(t, resp.GetThreads(), 1)
	assert.Empty(t, resp.GetThreads()[0].GetMessages())
	assert.Nil(t, resp.GetThreads()[0].GetDialog())
	assert.Equal(t, "300", resp.GetCursor())
}

// A failed delivery reloads the message like a reaction does: it comes back in its current
// state with the failures it still has, and the failed recipient is among the members.
func TestGetUpdates_Failures(t *testing.T) {
	thread, member := uuid.NewString(), uuid.New()
	failedID, recoveredID := uuid.New(), uuid.New()

	failed := func(msg uuid.UUID) journal.Event {
		return journal.Event{Kind: journal.KindMessageFailed, Fields: map[string]string{
			journal.FieldMsgID: msg.String(), journal.FieldActor: member.String(), journal.FieldErrCode: "403", journal.FieldErrMsg: "blocked",
		}}
	}
	updates := &fakeUpdates{
		changes: &journal.ContactChanges{Cursor: "300", After: 200, Threads: []string{thread}},
		events:  map[string][]journal.Event{thread: {failed(failedID), failed(failedID), failed(recoveredID)}},
		members: []journal.Member{{ID: "m1", ContactID: member.String()}},
	}
	history := &fakeHistory{byID: map[uuid.UUID]*model.Message{
		failedID: {ID: failedID, Body: "hi", Failures: []*model.DeliveryFailure{
			{MemberID: member, Code: "403", Message: "blocked"},
		}},
		// A later delivery cleared the failure before the client caught up.
		recoveredID: {ID: recoveredID, Body: "retried"},
	}}

	resp, err := NewUpdatesServer(history, updates, &fakeThreads{}).GetUpdates(context.Background(), updatesReq("200"))
	require.NoError(t, err)

	entry := resp.GetThreads()[0]
	require.Len(t, entry.GetMessages(), 2, "each failed message once, in its current state")

	got := entry.GetMessages()[0].GetFailures()
	require.Len(t, got, 1)
	assert.Equal(t, member.String(), got[0].GetMemberId())
	assert.Equal(t, "403", got[0].GetError().GetCode())
	assert.Equal(t, "blocked", got[0].GetError().GetMessage())
	assert.Empty(t, entry.GetMessages()[1].GetFailures(), "a recovered failure is not reported")

	require.Len(t, entry.GetMembers(), 1)
	assert.Equal(t, "m1", entry.GetMembers()[0].GetId())
}
