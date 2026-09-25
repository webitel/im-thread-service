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
	horizon   int64
	trimmed   int64
	changes   *journal.ContactChanges
	events    map[string][]journal.Event
	notMember map[string]bool
	unread    int64
}

func (f *fakeUpdates) ContactChanges(context.Context, string, string) (*journal.ContactChanges, error) {
	return f.changes, nil
}

func (f *fakeUpdates) ChangesSince(_ context.Context, threadID string, _, _ int64, limit int) ([]journal.Event, error) {
	ev := f.events[threadID]

	return ev[:min(len(ev), limit+1)], nil
}

func (f *fakeUpdates) SettledHorizon(context.Context) (int64, error) { return f.horizon, nil }

func (f *fakeUpdates) TrimHorizon(context.Context) (int64, error) { return f.trimmed, nil }

func (f *fakeUpdates) ReadStates(context.Context, string) ([]journal.ReadState, error) {
	return nil, nil
}

func (f *fakeUpdates) IsMember(_ context.Context, threadID, _ string, _ int32) (bool, error) {
	return !f.notMember[threadID], nil
}

func (f *fakeUpdates) Members(context.Context, string) ([]journal.Member, error) { return nil, nil }

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
	resp, err := NewUpdatesServer(&fakeHistory{}, &fakeUpdates{horizon: 77}, &fakeThreads{}).
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
	threads := &fakeThreads{byID: map[uuid.UUID]*model.Thread{newThread: {ID: newThread, Subject: "new dialog"}}}

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
		"more than 1000 threads": {horizon: 5, changes: &journal.ContactChanges{TooMany: true}},
		"cursor older than retention": {horizon: 5, trimmed: 200, changes: &journal.ContactChanges{
			After: 100, Threads: []string{thread},
		}},
		"more than 1000 changes": {horizon: 5, changes: &journal.ContactChanges{
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

type fakeHorizon struct{ horizon int64 }

func (f fakeHorizon) SettledHorizon(context.Context) (int64, error) { return f.horizon, nil }

// History hands out the GetUpdates cursor read before the page, so the client needs no second counter.
func TestSearchThreadMessagesHistory_ReturnsUpdatesCursor(t *testing.T) {
	srv := NewMessageHistoryServer(&fakeHistory{}, fakeHorizon{horizon: 92547098})

	resp, err := srv.SearchThreadMessagesHistory(context.Background(), &impb.SearchMessageHistoryRequest{
		ThreadId: uuid.NewString(), CallerId: uuid.NewString(), DomainId: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "92547098", resp.GetUpdatesCursor())
}

// Search hands out the GetUpdates cursor on the response, so the thread list and the catch-up agree.
func TestThreadSearch_ReturnsUpdatesCursor(t *testing.T) {
	srv := NewThreadService(&fakeThreads{}, nil, nil, fakeHorizon{horizon: 777})

	resp, err := srv.Search(context.Background(), &impb.ThreadSearchRequest{SelfId: uuid.NewString(), DomainIds: []int32{1}, Size: 10})
	require.NoError(t, err)
	assert.Equal(t, "777", resp.GetUpdatesCursor())
}
