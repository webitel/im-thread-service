package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type stubThreadTagStore struct {
	noopThreadTagStore

	addErr    error
	addExists bool

	removeThreadID uuid.UUID
	removeErr      error
	removedByName  bool

	names   []string
	cleared []uuid.UUID

	removeForCalls    int
	removeForThreadID uuid.UUID
	removeForContacts []uuid.UUID
	removeForTags     []string
}

func (s *stubThreadTagStore) Add(ctx context.Context, tag *model.ThreadTag) (*model.ThreadTag, bool, error) {
	if s.addErr != nil {
		return nil, false, s.addErr
	}

	if tag == nil {
		return nil, false, errors.InvalidArgument("tag cannot be nil")
	}

	tag.ID = uuid.New()

	return tag, !s.addExists, nil
}

func (s *stubThreadTagStore) Remove(context.Context, uuid.UUID, uuid.UUID) (uuid.UUID, error) {
	return s.removeThreadID, s.removeErr
}

func (s *stubThreadTagStore) RemoveByName(context.Context, uuid.UUID, uuid.UUID, string) (bool, error) {
	return s.removedByName, nil
}

func (s *stubThreadTagStore) RemoveForContacts(_ context.Context, threadID uuid.UUID, contactIDs []uuid.UUID, tags []string) ([]uuid.UUID, error) {
	s.removeForCalls++
	s.removeForThreadID = threadID
	s.removeForContacts = contactIDs
	s.removeForTags = tags

	return s.cleared, nil
}

func (s *stubThreadTagStore) ListNames(context.Context, uuid.UUID, uuid.UUID) ([]string, error) {
	return s.names, nil
}

func memberOf(threadID, contactID uuid.UUID, domainID int) *fakeThreadDialogStore {
	return &fakeThreadDialogStore{quickViewResult: []*model.ThreadDialog{{
		BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: domainID},
		ThreadID:  threadID,
		ContactID: contactID,
	}}}
}

func publishedTagEvents(t *testing.T, outbox *fakeOutboxStore) []*event.ThreadTagsChanged {
	t.Helper()

	out := make([]*event.ThreadTagsChanged, 0, len(outbox.published))
	for _, p := range outbox.published {
		e, ok := p.event.(*event.ThreadTagsChanged)
		require.True(t, ok, "unexpected event %T", p.event)
		assert.Equal(t, e.Topic(), p.topic)
		out = append(out, e)
	}

	return out
}

func TestThreadTagServiceAddTag(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()
	tagStore := &stubThreadTagStore{names: []string{"snoozed"}}
	outbox := &fakeOutboxStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore, threadDialogStore: memberOf(threadID, contactID, 7), outboxStore: outbox}
	svc := NewThreadTagService(uow, nil)

	result, err := svc.AddTag(context.Background(), &dto.AddTagRequest{ContactID: contactID, ThreadID: threadID, Tag: "snoozed"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "snoozed", result.Tag)

	events := publishedTagEvents(t, outbox)
	require.Len(t, events, 1)
	assert.Equal(t, threadID, events[0].ThreadID)
	assert.Equal(t, contactID, events[0].ContactID)
	assert.Equal(t, int32(7), events[0].DomainID)
	assert.Equal(t, []string{"snoozed"}, events[0].Tags)
	assert.Equal(t, []uuid.UUID{contactID}, events[0].Participants)
	assert.Equal(t, []uuid.UUID{contactID}, events[0].JournalAudience())
}

func TestThreadTagServiceAddTag_ExistingTagPublishesNothing(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()
	outbox := &fakeOutboxStore{}
	uow := fakeUnitOfWork{
		threadTagStore:    &stubThreadTagStore{addExists: true},
		threadDialogStore: memberOf(threadID, contactID, 1),
		outboxStore:       outbox,
	}

	result, err := NewThreadTagService(uow, nil).AddTag(context.Background(), &dto.AddTagRequest{ContactID: contactID, ThreadID: threadID, Tag: "snoozed"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, outbox.published)
}

func TestThreadTagServiceAddTag_NotMemberForbidden(t *testing.T) {
	outbox := &fakeOutboxStore{}
	uow := fakeUnitOfWork{threadTagStore: &stubThreadTagStore{}, threadDialogStore: &fakeThreadDialogStore{}, outboxStore: outbox}
	svc := NewThreadTagService(uow, nil)

	_, err := svc.AddTag(context.Background(), &dto.AddTagRequest{ContactID: uuid.New(), ThreadID: uuid.New(), Tag: "snoozed"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, errors.Code(err))
	assert.Empty(t, outbox.published)
}

func TestThreadTagServiceAddTagValidation(t *testing.T) {
	svc := NewThreadTagService(fakeUnitOfWork{threadTagStore: &stubThreadTagStore{}}, nil)

	tests := []struct {
		name string
		req  *dto.AddTagRequest
	}{
		{name: "nil request"},
		{name: "empty tag", req: &dto.AddTagRequest{ContactID: uuid.New(), ThreadID: uuid.New()}},
		{name: "no thread", req: &dto.AddTagRequest{ContactID: uuid.New(), Tag: "snoozed"}},
		{name: "no contact", req: &dto.AddTagRequest{ThreadID: uuid.New(), Tag: "snoozed"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddTag(context.Background(), tt.req)
			require.Error(t, err)
		})
	}
}

func TestThreadTagServiceRemoveTag(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()

	tests := []struct {
		name       string
		dialogs    *fakeThreadDialogStore
		wantEvents int
	}{
		{name: "member gets the change", dialogs: memberOf(threadID, contactID, 1), wantEvents: 1},
		{name: "former member gets nothing", dialogs: &fakeThreadDialogStore{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbox := &fakeOutboxStore{}
			uow := fakeUnitOfWork{
				threadTagStore:    &stubThreadTagStore{removeThreadID: threadID},
				threadDialogStore: tt.dialogs,
				outboxStore:       outbox,
			}

			err := NewThreadTagService(uow, nil).RemoveTag(context.Background(), &dto.RemoveTagRequest{TagID: uuid.New(), ContactID: contactID})
			require.NoError(t, err)
			assert.Len(t, publishedTagEvents(t, outbox), tt.wantEvents)
		})
	}
}

func TestThreadTagServiceRemoveTag_NotFound(t *testing.T) {
	outbox := &fakeOutboxStore{}
	uow := fakeUnitOfWork{
		threadTagStore: &stubThreadTagStore{removeErr: errors.NotFound("tag not found")},
		outboxStore:    outbox,
	}

	err := NewThreadTagService(uow, nil).RemoveTag(context.Background(), &dto.RemoveTagRequest{TagID: uuid.New(), ContactID: uuid.New()})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, errors.Code(err))
	assert.Empty(t, outbox.published)
}

func TestThreadTagServiceRemoveTagByName(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()

	tests := []struct {
		name       string
		removed    bool
		wantEvents int
	}{
		{name: "existing tag", removed: true, wantEvents: 1},
		{name: "absent tag is a no-op", removed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbox := &fakeOutboxStore{}
			uow := fakeUnitOfWork{
				threadTagStore:    &stubThreadTagStore{removedByName: tt.removed},
				threadDialogStore: memberOf(threadID, contactID, 1),
				outboxStore:       outbox,
			}

			err := NewThreadTagService(uow, nil).RemoveTagByName(context.Background(), &dto.RemoveTagByNameRequest{
				ContactID: contactID, ThreadID: threadID, Tag: "snoozed",
			})
			require.NoError(t, err)
			assert.Len(t, publishedTagEvents(t, outbox), tt.wantEvents)
		})
	}
}

func TestThreadTagServiceOnMessage(t *testing.T) {
	threadID := uuid.New()
	sender, cleared, untouched := uuid.New(), uuid.New(), uuid.New()
	everyone := []*model.ThreadDialog{{ContactID: sender}, {ContactID: cleared}, {ContactID: untouched}}

	tests := []struct {
		name           string
		msg            *model.Message
		wantRecipients []uuid.UUID
		wantEvents     int
	}{
		{
			name:           "content message clears snoozed for everyone but the sender",
			msg:            &model.Message{ThreadID: threadID, Type: model.MessageTypeText, DomainID: 3, From: shared.Peer{ID: sender}, To: everyone},
			wantRecipients: []uuid.UUID{cleared, untouched},
			wantEvents:     1,
		},
		{
			name:           "system message clears snoozed too",
			msg:            &model.Message{ThreadID: threadID, Type: model.MessageTypeSystem, DomainID: 3, From: shared.Peer{ID: sender}, To: everyone},
			wantRecipients: []uuid.UUID{cleared, untouched},
			wantEvents:     1,
		},
		{
			name: "own message alone touches nothing",
			msg:  &model.Message{ThreadID: threadID, Type: model.MessageTypeText, From: shared.Peer{ID: sender}, To: everyone[:1]},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagStore := &stubThreadTagStore{cleared: []uuid.UUID{cleared}}
			outbox := &fakeOutboxStore{}
			uow := fakeUnitOfWork{threadTagStore: tagStore, outboxStore: outbox}

			require.NoError(t, NewThreadTagService(uow, nil).OnMessage(context.Background(), uow, tt.msg))

			events := publishedTagEvents(t, outbox)
			require.Len(t, events, tt.wantEvents)

			if tt.wantRecipients == nil {
				assert.Zero(t, tagStore.removeForCalls)

				return
			}

			assert.Equal(t, threadID, tagStore.removeForThreadID)
			assert.Equal(t, tt.wantRecipients, tagStore.removeForContacts)
			assert.Equal(t, []string{"snoozed"}, tagStore.removeForTags)
			assert.Equal(t, cleared, events[0].ContactID)
			assert.Equal(t, int32(3), events[0].DomainID)
		})
	}
}

func TestThreadTagServiceOnMemberLeft_RemovesAllTags(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()
	tagStore := &stubThreadTagStore{}
	uow := fakeUnitOfWork{threadTagStore: tagStore}

	require.NoError(t, NewThreadTagService(uow, nil).OnMemberLeft(context.Background(), uow, threadID, contactID))

	assert.Equal(t, 1, tagStore.removeForCalls)
	assert.Equal(t, threadID, tagStore.removeForThreadID)
	assert.Equal(t, []uuid.UUID{contactID}, tagStore.removeForContacts)
	assert.Empty(t, tagStore.removeForTags)
}

func TestThreadTagServiceListTags(t *testing.T) {
	uow := fakeUnitOfWork{threadTagStore: &stubThreadTagStore{names: []string{"snoozed", "vip"}}}

	tags, err := NewThreadTagService(uow, nil).ListTags(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, []string{"snoozed", "vip"}, tags)
}

func TestThreadTagServiceSearchTags(t *testing.T) {
	svc := NewThreadTagService(fakeUnitOfWork{threadTagStore: &stubThreadTagStore{}}, nil)

	result, err := svc.SearchTags(context.Background(), &dto.SearchTagsRequest{ContactID: uuid.New(), Page: 1, Size: 20})
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestThreadTagServiceSearchTagsValidation(t *testing.T) {
	svc := NewThreadTagService(fakeUnitOfWork{threadTagStore: &stubThreadTagStore{}}, nil)

	_, err := svc.SearchTags(context.Background(), nil)
	require.Error(t, err)

	_, err = svc.SearchTags(context.Background(), &dto.SearchTagsRequest{ContactID: uuid.Nil, Page: 1, Size: 20})
	require.Error(t, err)
}

func TestThreadTagServiceSnooze(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()
	outbox := &fakeOutboxStore{}
	uow := fakeUnitOfWork{
		threadTagStore:    &stubThreadTagStore{names: []string{SnoozedTag}},
		threadDialogStore: memberOf(threadID, contactID, 1),
		outboxStore:       outbox,
	}

	tag, err := NewThreadTagService(uow, nil).Snooze(context.Background(), threadID, contactID)
	require.NoError(t, err)
	assert.Equal(t, SnoozedTag, tag.Tag)
	assert.Equal(t, threadID, tag.ThreadID)
	assert.Equal(t, contactID, tag.ContactID)

	events := publishedTagEvents(t, outbox)
	require.Len(t, events, 1)
	assert.Equal(t, []string{SnoozedTag}, events[0].Tags)
}

func TestThreadTagServiceSnooze_NotMemberForbidden(t *testing.T) {
	uow := fakeUnitOfWork{threadTagStore: &stubThreadTagStore{}, threadDialogStore: &fakeThreadDialogStore{}, outboxStore: &fakeOutboxStore{}}

	_, err := NewThreadTagService(uow, nil).Snooze(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, errors.Code(err))
}

func TestThreadTagServiceUnsnooze(t *testing.T) {
	threadID, contactID := uuid.New(), uuid.New()

	tests := []struct {
		name       string
		snoozed    bool
		wantEvents int
	}{
		{name: "snoozed thread returns to Active", snoozed: true, wantEvents: 1},
		{name: "not snoozed is a no-op", snoozed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbox := &fakeOutboxStore{}
			uow := fakeUnitOfWork{
				threadTagStore:    &stubThreadTagStore{removedByName: tt.snoozed},
				threadDialogStore: memberOf(threadID, contactID, 1),
				outboxStore:       outbox,
			}

			require.NoError(t, NewThreadTagService(uow, nil).Unsnooze(context.Background(), threadID, contactID))
			assert.Len(t, publishedTagEvents(t, outbox), tt.wantEvents)
		})
	}
}
