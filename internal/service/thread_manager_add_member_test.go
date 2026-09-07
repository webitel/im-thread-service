package service

import (
	"context"
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
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type fakeUnitOfWork struct {
	threadDialogStore    store.ThreadDialogStore
	messageStore         store.MessageStore
	messageExternalStore store.MessageExternalStore
	messageStatusStore   store.MessageStatusStore
	messageReactionStore store.MessageReactionStore
	outboxStore          store.OutboxStore
	botControlStore      store.BotControlStore
	threadStore          store.ThreadStore
	threadTagStore       store.ThreadTagStore
	threadVariablesStore store.ThreadVariablesStore
}

func (f fakeUnitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context, store.UnitOfWork) error) error {
	return fn(ctx, f)
}

func (f fakeUnitOfWork) ThreadDialogStore() store.ThreadDialogStore {
	return f.threadDialogStore
}

func (f fakeUnitOfWork) ThreadStore() store.ThreadStore { return f.threadStore }

func (f fakeUnitOfWork) ThreadPermissionStore() store.ThreadPermissionStore {
	return nil
}

func (f fakeUnitOfWork) MessageHistory() store.MessageHistory {
	return nil
}

func (f fakeUnitOfWork) MessageRevisions() store.MessageRevisionStore {
	return nil
}

func (f fakeUnitOfWork) Messages() store.MessageStore {
	return f.messageStore
}

func (f fakeUnitOfWork) MessageExternal() store.MessageExternalStore {
	return f.messageExternalStore
}

func (f fakeUnitOfWork) MessageStatuses() store.MessageStatusStore {
	if f.messageStatusStore == nil {
		return noopMessageStatusStore{}
	}

	return f.messageStatusStore
}

type noopMessageStatusStore struct{}

func (noopMessageStatusStore) InsertSent(context.Context, *model.Message, []uuid.UUID) error {
	return nil
}

func (noopMessageStatusStore) MarkDelivered(context.Context, []*model.StatusReceipt) ([]*model.StatusChange, error) {
	return nil, nil
}

func (noopMessageStatusStore) MarkRead(context.Context, []*model.ReadReceipt) ([]*model.StatusChange, error) {
	return nil, nil
}

func (noopMessageStatusStore) MarkFailed(context.Context, []*model.StatusReceipt) ([]*model.StatusChange, error) {
	return nil, nil
}

func (noopMessageStatusStore) ReadUnread(context.Context, int32, uuid.UUID, []uuid.UUID) (map[uuid.UUID]int64, error) {
	return make(map[uuid.UUID]int64), nil
}

func (noopMessageStatusStore) UnreadSummary(context.Context, int32, uuid.UUID) (model.UnreadSummary, error) {
	return model.UnreadSummary{}, nil
}

func (noopMessageStatusStore) ReconcileUnread(context.Context, int32) (int64, error) {
	return 0, nil
}

func (f fakeUnitOfWork) Outbox() store.OutboxStore {
	return f.outboxStore
}

func (f fakeUnitOfWork) InteractiveCallback() store.InteractiveCallback {
	return nil
}

func (f fakeUnitOfWork) MessageReactions() store.MessageReactionStore {
	return f.messageReactionStore
}

func (f fakeUnitOfWork) BotControl() store.BotControlStore {
	return f.botControlStore
}

func (f fakeUnitOfWork) ThreadTagStore() store.ThreadTagStore {
	if f.threadTagStore == nil {
		return noopThreadTagStore{}
	}

	return f.threadTagStore
}

func (f fakeUnitOfWork) ThreadVariables() store.ThreadVariablesStore {
	if f.threadVariablesStore == nil {
		return noopThreadVariablesStore{}
	}

	return f.threadVariablesStore
}

type noopThreadVariablesStore struct{}

func (noopThreadVariablesStore) Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error) {
	return variables.Variables, nil
}

func (noopThreadVariablesStore) Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error) {
	return model.Page[*model.ThreadVariables]{}, nil
}

func (noopThreadVariablesStore) Locate(ctx context.Context, threadID uuid.UUID) (*model.ThreadVariables, error) {
	return nil, nil
}

func (noopThreadVariablesStore) Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error) {
	return nil, nil
}

type noopThreadTagStore struct{}

func (noopThreadTagStore) Add(ctx context.Context, tag *model.ThreadTag) (*model.ThreadTag, error) {
	return tag, nil
}

func (noopThreadTagStore) Remove(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (noopThreadTagStore) ListForContact(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]*model.ThreadTag, error) {
	return make(map[uuid.UUID][]*model.ThreadTag), nil
}

func (noopThreadTagStore) SearchTags(context.Context, uuid.UUID, int, int) ([]string, error) {
	return nil, nil
}

type fakeThreadDialogStore struct {
	fullViewResult  []*model.ThreadDialogExtended
	lastFilter      *model.ThreadDialogStoreFilter
	initiatorPair   *model.ThreadDialogExtended
	targetPair      *model.ThreadDialogExtended
	lastDeleteID    uuid.UUID
	lastReason      *string
	lastCreate      *model.ThreadDialogExtended
	quickViewResult []*model.ThreadDialog
	quickViewCalls  int
}

func (f *fakeThreadDialogStore) Create(ctx context.Context, threadDialog *model.ThreadDialogExtended) (*model.ThreadDialogExtended, error) {
	f.lastCreate = threadDialog
	if threadDialog == nil {
		return nil, errors.InvalidArgument("received nil pointer thread dialog")
	}

	if threadDialog.ID == uuid.Nil {
		threadDialog.ID = uuid.New()
	}

	return threadDialog, nil
}

func (f *fakeThreadDialogStore) Delete(ctx context.Context, memberID uuid.UUID, leaveReason *string) error {
	f.lastDeleteID = memberID
	f.lastReason = leaveReason

	return nil
}

func (f *fakeThreadDialogStore) GetQuickView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error) {
	f.lastFilter = filter
	f.quickViewCalls++

	return f.quickViewResult, nil
}

func (f *fakeThreadDialogStore) GetFullView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialogExtended, error) {
	f.lastFilter = filter

	return f.fullViewResult, nil
}

func (f *fakeThreadDialogStore) FindActorsPair(ctx context.Context, initiatorsContact, targetMember uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error) {
	return f.initiatorPair, f.targetPair, nil
}

type publishedOutboxEvent struct {
	topic string
	event event.Outboxer
}

var _ store.ThreadStore = (*fakeThreadStore)(nil)

type fakeThreadStore struct {
	getResult       *model.Thread
	getError        error
	lastQueryObject queryobject.QueryObject
}

func (f *fakeThreadStore) ResolveThread(ctx context.Context, q model.ResolveThreadQuery) (*model.Thread, error) {
	panic("unimplemented")
}

func (f *fakeThreadStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	panic("unimplemented")
}

func (f *fakeThreadStore) SearchLeft(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	panic("unimplemented")
}

func (f *fakeThreadStore) Get(ctx context.Context, qo queryobject.QueryObject) (*model.Thread, error) {
	f.lastQueryObject = qo
	if f.getError != nil {
		return nil, f.getError
	}

	return f.getResult, nil
}

func (f *fakeThreadStore) Create(ctx context.Context, thread *model.Thread) (*model.Thread, error) {
	return nil, errors.Unavailable("method unimplemented")
}

func (f *fakeThreadStore) Update(ctx context.Context, thread *model.Thread) error {
	return nil
}

type fakeOutboxStore struct {
	published []publishedOutboxEvent
}

func (f *fakeOutboxStore) Publish(ctx context.Context, topic string, event event.Outboxer) error {
	f.published = append(f.published, publishedOutboxEvent{topic: topic, event: event})

	return nil
}

func (f *fakeOutboxStore) Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error) {
	return 0, nil
}

type fakeMessageStore struct {
	lastSavedSystemMessage *model.Message
	replyPreview           *model.ReplyToPreview
	replyPreviewErr        error

	editMessageErr    error
	editMessageCalls  int
	lastEditedMessage *model.Message

	deleteMessagesResult *model.MessageDeleteResult
	deleteMessagesErr    error
	deleteMessagesCalls  int
	lastDeletedIDs       []uuid.UUID
	lastDeleterID        uuid.UUID

	forwardSources    []*model.Message
	forwardSourcesErr error
	lastForwardIDs    []uuid.UUID
	lastForwardCaller uuid.UUID

	copyAttachmentsErr   error
	copiedAttachmentsFor []uuid.UUID
	savedMessages        []*model.Message
}

func (f *fakeMessageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if msg.ID == uuid.Nil {
		msg.ID = uuid.Must(uuid.NewV7())
	}

	f.savedMessages = append(f.savedMessages, msg)

	return msg, nil
}

func (f *fakeMessageStore) LoadForwardSources(
	ctx context.Context,
	ids []uuid.UUID,
	callerID uuid.UUID,
	domainID int32,
) ([]*model.Message, error) {
	f.lastForwardIDs = ids
	f.lastForwardCaller = callerID

	if f.forwardSourcesErr != nil {
		return nil, f.forwardSourcesErr
	}

	return f.forwardSources, nil
}

func (f *fakeMessageStore) CopyAttachments(ctx context.Context, sourceID, targetID uuid.UUID) error {
	if f.copyAttachmentsErr != nil {
		return f.copyAttachmentsErr
	}

	f.copiedAttachmentsFor = append(f.copiedAttachmentsFor, sourceID)

	return nil
}

func (f *fakeMessageStore) GetReplyPreview(ctx context.Context, id uuid.UUID, domainID int32) (*model.ReplyToPreview, error) {
	if f.replyPreviewErr != nil {
		return nil, f.replyPreviewErr
	}

	if f.replyPreview == nil {
		return nil, store.ErrReplyTargetNotFound
	}

	return f.replyPreview, nil
}

func (f *fakeMessageStore) SaveImages(ctx context.Context, messageID uuid.UUID, images []*model.MessageImage) ([]*model.MessageImage, error) {
	return images, nil
}

func (f *fakeMessageStore) SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error) {
	return docs, nil
}

func (f *fakeMessageStore) ReadMessage(ctx context.Context, read struct {
	DomainID  int32
	ThreadID  uuid.UUID
	MessageID uuid.UUID
	UserID    uuid.UUID
},
) error {
	return nil
}

func (f *fakeMessageStore) SaveMessageContact(ctx context.Context, msg *model.Message) (*model.Message, error) {
	return msg, nil
}

func (f *fakeMessageStore) SaveMessageLocation(ctx context.Context, msg *model.Message) (*model.Message, error) {
	return msg, nil
}

func (f *fakeMessageStore) SaveInteractiveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	return msg, nil
}

func (f *fakeMessageStore) EditMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	f.editMessageCalls++
	f.lastEditedMessage = msg

	if f.editMessageErr != nil {
		return nil, f.editMessageErr
	}

	return msg, nil
}

func (f *fakeMessageStore) DeleteMessages(ctx context.Context, ids []uuid.UUID, deleterID uuid.UUID) (*model.MessageDeleteResult, error) {
	f.deleteMessagesCalls++
	f.lastDeletedIDs = ids
	f.lastDeleterID = deleterID

	if f.deleteMessagesErr != nil {
		return nil, f.deleteMessagesErr
	}

	return f.deleteMessagesResult, nil
}

func (f *fakeMessageStore) SaveSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	f.lastSavedSystemMessage = msg
	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}

	now := time.Now().UTC()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}

	if msg.UpdatedAt.IsZero() {
		msg.UpdatedAt = now
	}

	return msg, nil
}

type fakePrivacyChecker struct{}

func (f fakePrivacyChecker) CanInvite(ctx context.Context, initiatorID, targetID uuid.UUID) error {
	return nil
}

func (f fakePrivacyChecker) CanSend(ctx context.Context, senderID, recipientID uuid.UUID) error {
	return nil
}

func TestFindAddMemberActors_ResolvesInitiatorAndTargetByContactID(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	targetContactID := uuid.New()
	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{ContactID: initiatorContactID, ThreadID: threadID},
			{ContactID: targetContactID, ThreadID: threadID},
		},
	}
	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{threadDialogStore: threadDialogStore},
	}

	initiator, target, err := svc.findAddMemberActors(context.Background(), threadID, initiatorContactID, targetContactID)

	require.NoError(t, err)
	require.NotNil(t, initiator)
	require.NotNil(t, target)
	require.Equal(t, initiatorContactID, initiator.ContactID)
	require.Equal(t, targetContactID, target.ContactID)
	require.NotNil(t, threadDialogStore.lastFilter)
	require.Equal(t, []uuid.UUID{threadID}, threadDialogStore.lastFilter.ThreadIDs)
	require.Equal(t, []uuid.UUID{initiatorContactID, targetContactID}, threadDialogStore.lastFilter.ContactIDs)
}

func TestRemoveMember_ForwardsLeaveReasonToStoreDelete(t *testing.T) {
	targetID := uuid.New()
	threadID := uuid.New()
	reason := "left by own decision"

	threadDialogStore := &fakeThreadDialogStore{
		targetPair: &model.ThreadDialogExtended{
			BaseModel: shared.BaseModel{ID: targetID, DomainID: 1},
			ThreadID:  threadID,
			ContactID: uuid.New(),
		},
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: targetID}, ContactID: uuid.New(), ThreadID: threadID, ThreadRole: model.RoleMember},
		},
	}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{threadDialogStore: threadDialogStore, messageStore: messageStore, outboxStore: outboxStore},
	}

	err := svc.RemoveMember(context.Background(), &dto.RemoveMemberRequest{
		TargetMemberID:     targetID,
		InitiatorContactID: uuid.Nil,
		Reason:             &reason,
	})

	require.NoError(t, err)
	require.Equal(t, targetID, threadDialogStore.lastDeleteID)
	require.NotNil(t, threadDialogStore.lastReason)
	require.Equal(t, reason, *threadDialogStore.lastReason)
}

func TestRemoveMember_SendsSystemMessageAndRetainsRemovedRecipient(t *testing.T) {
	initiatorMemberID := uuid.New()
	initiatorContactID := uuid.New()
	targetID := uuid.New()
	targetContactID := uuid.New()
	otherContactID := uuid.New()
	threadID := uuid.New()
	reason := "left by own decision"

	threadDialogStore := &fakeThreadDialogStore{
		initiatorPair: &model.ThreadDialogExtended{
			BaseModel:  shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
			ContactID:  initiatorContactID,
			ThreadID:   threadID,
			ThreadRole: model.RoleOwner,
			Permissions: model.ThreadPermissions{
				CanAddMembers:    true,
				CanRemoveMembers: true,
			},
		},
		targetPair: &model.ThreadDialogExtended{
			BaseModel:  shared.BaseModel{ID: targetID, DomainID: 1},
			ContactID:  targetContactID,
			ThreadID:   threadID,
			ThreadRole: model.RoleMember,
		},
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: initiatorMemberID}, ContactID: initiatorContactID, ThreadID: threadID, ThreadRole: model.RoleOwner},
			{BaseModel: shared.BaseModel{ID: targetID}, ContactID: targetContactID, ThreadID: threadID, ThreadRole: model.RoleMember},
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: otherContactID, ThreadID: threadID, ThreadRole: model.RoleMember},
		},
	}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      messageStore,
			outboxStore:       outboxStore,
		},
	}

	err := svc.RemoveMember(context.Background(), &dto.RemoveMemberRequest{
		TargetMemberID:     targetID,
		InitiatorContactID: initiatorContactID,
		Reason:             &reason,
	})

	require.NoError(t, err)
	require.Equal(t, targetID, threadDialogStore.lastDeleteID)
	require.NotNil(t, messageStore.lastSavedSystemMessage)
	require.Equal(t, threadID, messageStore.lastSavedSystemMessage.ThreadID)
	require.Equal(t, model.MessageTypeSystem, messageStore.lastSavedSystemMessage.Type)
	require.NotNil(t, messageStore.lastSavedSystemMessage.System)
	require.Equal(t, memberRemovedSystemMessageType, messageStore.lastSavedSystemMessage.System.Type)
	require.Equal(t, targetID, messageStore.lastSavedSystemMessage.System.Metadata["removed_member_id"])
	require.Equal(t, targetContactID, messageStore.lastSavedSystemMessage.System.Metadata["removed_member_contact_id"])
	require.Equal(t, threadID, messageStore.lastSavedSystemMessage.System.Metadata["thread_id"])
	require.Equal(t, reason, messageStore.lastSavedSystemMessage.System.Metadata["reason"])
	require.Len(t, messageStore.lastSavedSystemMessage.To, 3)
	require.Equal(t, targetContactID, messageStore.lastSavedSystemMessage.To[1].ContactID)
	require.NotNil(t, messageStore.lastSavedSystemMessage.Member)
	require.Equal(t, initiatorMemberID, messageStore.lastSavedSystemMessage.Member.ID)
	require.GreaterOrEqual(t, len(outboxStore.published), 1)
	require.NotNil(t, threadDialogStore.lastFilter)
	require.Equal(t, []uuid.UUID{threadID}, threadDialogStore.lastFilter.ThreadIDs)
}

func TestAddMember_SetsInvitedByFromInitiatorDialogID(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	newMemberContactID := uuid.New()
	initiatorMemberID := uuid.New()

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:  shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:  initiatorContactID,
				ThreadID:   threadID,
				ThreadRole: model.RoleOwner,
				Permissions: model.ThreadPermissions{
					CanAddMembers: true,
				},
				Settings: model.BaseThreadSetting{Title: "Initiator title"},
			},
		},
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: initiatorMemberID}, ContactID: initiatorContactID, ThreadID: threadID, ThreadRole: model.RoleOwner},
		},
	}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow:            fakeUnitOfWork{threadDialogStore: threadDialogStore, messageStore: messageStore, outboxStore: outboxStore},
		privacyChecker: fakePrivacyChecker{},
	}

	_, err := svc.AddMember(context.Background(), &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: newMemberContactID,
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
	})

	require.NoError(t, err)
	require.NotNil(t, threadDialogStore.lastCreate)
	require.NotNil(t, threadDialogStore.lastCreate.InvitedBy)
	require.Equal(t, initiatorMemberID, *threadDialogStore.lastCreate.InvitedBy)
}

func TestTransfer_AddsMemberRemovesInitiatorAndSendsTransferSystemMessage(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	initiatorMemberID := uuid.New()
	newMemberContactID := uuid.New()
	otherContactID := uuid.New()

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:  shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:  initiatorContactID,
				ThreadID:   threadID,
				ThreadRole: model.RoleOwner,
				Permissions: model.ThreadPermissions{
					CanAddMembers: true,
				},
				Settings: model.BaseThreadSetting{Title: "Thread title"},
			},
		},
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: newMemberContactID, ThreadID: threadID, ThreadRole: model.RoleMember},
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ContactID: otherContactID, ThreadID: threadID, ThreadRole: model.RoleMember},
		},
	}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow:            fakeUnitOfWork{threadDialogStore: threadDialogStore, messageStore: messageStore, outboxStore: outboxStore},
		privacyChecker: fakePrivacyChecker{},
	}

	newMemberID, err := svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           threadID,
		NewMemberContactID: newMemberContactID,
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, newMemberID)
	require.NotNil(t, threadDialogStore.lastCreate)
	require.NotNil(t, threadDialogStore.lastCreate.InvitedBy)
	require.Equal(t, initiatorMemberID, *threadDialogStore.lastCreate.InvitedBy)
	require.Equal(t, initiatorMemberID, threadDialogStore.lastDeleteID)
	require.NotNil(t, threadDialogStore.lastReason)
	require.Equal(t, memberTransferedLeaveReason, *threadDialogStore.lastReason)
	require.NotNil(t, messageStore.lastSavedSystemMessage)
	require.NotNil(t, messageStore.lastSavedSystemMessage.System)
	require.Equal(t, memberTransferedSystemMessageType, messageStore.lastSavedSystemMessage.System.Type)
	require.Equal(t, initiatorMemberID, messageStore.lastSavedSystemMessage.System.Metadata["transferred_member_id"])
	require.Equal(t, newMemberID, messageStore.lastSavedSystemMessage.System.Metadata["new_member_id"])
	require.GreaterOrEqual(t, len(outboxStore.published), 1)
}

func TestTransfer_ReturnsValidationErrorWhenInitiatorIsNil(t *testing.T) {
	svc := &ThreadManagementService{}

	_, err := svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           uuid.New(),
		NewMemberContactID: uuid.New(),
		InitiatorContactID: uuid.Nil,
		NewMemberRole:      model.RoleMember,
	})

	require.Error(t, err)
}

var (
	_ store.UnitOfWork        = fakeUnitOfWork{}
	_ store.ThreadDialogStore = (*fakeThreadDialogStore)(nil)
	_ store.ThreadStore       = nil
	_ store.OutboxStore       = (*fakeOutboxStore)(nil)
	_ store.MessageStore      = (*fakeMessageStore)(nil)
	_                         = dto.AddMemberRequest{}
)
