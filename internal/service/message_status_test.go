package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type recordingMessageStatusStore struct {
	insertedMsg        *model.Message
	insertedRecipients []uuid.UUID
	deliveredReceipts  []*model.StatusReceipt

	changes []*model.StatusChange
}

func (r *recordingMessageStatusStore) InsertSent(_ context.Context, msg *model.Message, recipientIDs []uuid.UUID) error {
	r.insertedMsg = msg
	r.insertedRecipients = recipientIDs

	return nil
}

func (r *recordingMessageStatusStore) MarkDelivered(_ context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error) {
	r.deliveredReceipts = append(r.deliveredReceipts, receipts...)

	return r.changes, nil
}

func (r *recordingMessageStatusStore) MarkRead(context.Context, []*model.ReadReceipt) ([]*model.StatusChange, error) {
	return r.changes, nil
}

func (r *recordingMessageStatusStore) MarkFailed(context.Context, []*model.StatusReceipt) ([]*model.StatusChange, error) {
	return r.changes, nil
}

func (r *recordingMessageStatusStore) ReadUnread(context.Context, int32, uuid.UUID, []uuid.UUID) (map[uuid.UUID]int64, error) {
	return make(map[uuid.UUID]int64), nil
}

func (r *recordingMessageStatusStore) UnreadSummary(context.Context, int32, uuid.UUID) (model.UnreadSummary, error) {
	return model.UnreadSummary{}, nil
}

func (r *recordingMessageStatusStore) ReconcileUnread(context.Context, int32) (int64, error) {
	return 0, nil
}

func TestGroupStatusChanges_BatchesSameRecipientAndStatus(t *testing.T) {
	var (
		threadID = uuid.New()
		memberID = uuid.New()
		msgA     = uuid.New()
		msgB     = uuid.New()
		early    = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
		late     = early.Add(time.Minute)
	)

	events := groupStatusChanges([]*model.StatusChange{
		{ThreadID: threadID, MemberID: memberID, MessageID: msgA, Status: model.MessageDeliveryStatusRead, Via: ptr("ws"), UpdatedAt: late},
		{ThreadID: threadID, MemberID: memberID, MessageID: msgB, Status: model.MessageDeliveryStatusRead, Via: ptr("ws"), UpdatedAt: early},
		nil,
	})

	require.Len(t, events, 1)
	require.Equal(t, []uuid.UUID{msgA, msgB}, events[0].MessageIDs)
	require.Equal(t, "read", events[0].Status)
	require.Equal(t, "ws", events[0].Via)
	require.Equal(t, late, events[0].OccurredAt)
}

func TestGroupStatusChanges_SplitsByMemberStatusAndVia(t *testing.T) {
	var (
		threadID = uuid.New()
		memberA  = uuid.New()
		memberB  = uuid.New()
	)

	events := groupStatusChanges([]*model.StatusChange{
		{ThreadID: threadID, MemberID: memberA, MessageID: uuid.New(), Status: model.MessageDeliveryStatusDelivered, Via: ptr("ws")},
		{ThreadID: threadID, MemberID: memberA, MessageID: uuid.New(), Status: model.MessageDeliveryStatusDelivered, Via: ptr("push")},
		{ThreadID: threadID, MemberID: memberB, MessageID: uuid.New(), Status: model.MessageDeliveryStatusDelivered, Via: ptr("ws")},
		{ThreadID: threadID, MemberID: memberA, MessageID: uuid.New(), Status: model.MessageDeliveryStatusRead, Via: ptr("ws")},
	})

	require.Len(t, events, 4)
}

func TestGroupStatusChanges_FailedAreNotBatched(t *testing.T) {
	var (
		threadID = uuid.New()
		memberID = uuid.New()
	)

	events := groupStatusChanges([]*model.StatusChange{
		{ThreadID: threadID, MemberID: memberID, MessageID: uuid.New(), Status: model.MessageDeliveryStatusFailed, Error: map[string]any{"code": "1"}},
		{ThreadID: threadID, MemberID: memberID, MessageID: uuid.New(), Status: model.MessageDeliveryStatusFailed, Error: map[string]any{"code": "2"}},
	})

	require.Len(t, events, 2)
	require.Equal(t, "failed", events[0].Status)
	require.Equal(t, map[string]any{"code": "1"}, events[0].Error)
	require.Equal(t, map[string]any{"code": "2"}, events[1].Error)
}

func TestMessageStatusService_MarkDelivered_PublishesStatusEvents(t *testing.T) {
	var (
		threadID = uuid.New()
		memberID = uuid.New()
	)

	statusStore := &recordingMessageStatusStore{
		changes: []*model.StatusChange{
			{ThreadID: threadID, MemberID: memberID, MessageID: uuid.New(), Status: model.MessageDeliveryStatusDelivered, Via: ptr("ws")},
			{ThreadID: threadID, MemberID: memberID, MessageID: uuid.New(), Status: model.MessageDeliveryStatusDelivered, Via: ptr("ws")},
		},
	}
	outboxStore := &fakeOutboxStore{}
	dialogStore := &fakeThreadDialogStore{
		quickViewResult: []*model.ThreadDialog{
			{ThreadID: threadID, ContactID: memberID},
			{ThreadID: threadID, ContactID: uuid.New()},
		},
	}

	svc := NewMessageStatusService(fakeUnitOfWork{
		messageStatusStore: statusStore,
		outboxStore:        outboxStore,
		threadDialogStore:  dialogStore,
	}, slog.Default())

	updated, err := svc.MarkDelivered(context.Background(), []*model.StatusReceipt{
		{ThreadID: threadID, MemberID: memberID, MessageID: uuid.New()},
	})

	require.NoError(t, err)
	require.EqualValues(t, 2, updated)
	require.Len(t, outboxStore.published, 1)
	require.Equal(t, "im_message."+threadID.String()+".message.status.v1", outboxStore.published[0].topic)

	statusEvent, ok := outboxStore.published[0].event.(*event.MessageStatusChanged)
	require.True(t, ok)
	require.Equal(t, "delivered", statusEvent.Status)
	require.Len(t, statusEvent.MessageIDs, 2)
	require.Len(t, statusEvent.Participants, 2)
	require.Equal(t, []uuid.UUID{threadID}, dialogStore.lastFilter.ThreadIDs)
}

func TestMessageStatusService_MarkRead_NoChangesPublishesNothing(t *testing.T) {
	outboxStore := &fakeOutboxStore{}

	svc := NewMessageStatusService(fakeUnitOfWork{
		messageStatusStore: &recordingMessageStatusStore{},
		outboxStore:        outboxStore,
	}, slog.Default())

	updated, err := svc.MarkRead(context.Background(), []*model.ReadReceipt{
		{ThreadID: uuid.New(), MemberID: uuid.New(), UpToMessageID: uuid.New()},
	})

	require.NoError(t, err)
	require.Zero(t, updated)
	require.Empty(t, outboxStore.published)
}

func TestInsertSentStatuses_ExcludesEffectiveSender(t *testing.T) {
	var (
		sender    = uuid.New()
		recipient = uuid.New()
		bot       = uuid.New()
	)

	statusStore := &recordingMessageStatusStore{}
	uow := fakeUnitOfWork{messageStatusStore: statusStore}

	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		DomainID: 1,
		From:     shared.Peer{ID: sender},
		To: []*model.ThreadDialog{
			{ContactID: sender},
			{ContactID: recipient},
			{ContactID: bot, IsBot: true},
			nil,
		},
	}

	svc := &MessageService{}
	require.NoError(t, svc.insertSentStatuses(context.Background(), uow, msg))

	require.Equal(t, msg, statusStore.insertedMsg)
	require.Equal(t, []uuid.UUID{recipient, bot}, statusStore.insertedRecipients)
}

func TestInsertSentStatuses_BotPromotedToDelivered(t *testing.T) {
	var (
		sender = uuid.New()
		human  = uuid.New()
		bot    = uuid.New()
	)

	statusStore := &recordingMessageStatusStore{}
	uow := fakeUnitOfWork{messageStatusStore: statusStore}

	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		DomainID: 2,
		From:     shared.Peer{ID: sender},
		To: []*model.ThreadDialog{
			{ContactID: sender},
			{ContactID: human},
			{ContactID: bot, IsBot: true},
		},
	}

	svc := &MessageService{}
	require.NoError(t, svc.insertSentStatuses(context.Background(), uow, msg))

	// Everyone but the sender gets SENT; only the bot is promoted.
	require.Equal(t, []uuid.UUID{human, bot}, statusStore.insertedRecipients)
	require.Len(t, statusStore.deliveredReceipts, 1)

	receipt := statusStore.deliveredReceipts[0]
	require.Equal(t, bot, receipt.MemberID)
	require.Equal(t, msg.ID, receipt.MessageID)
	require.Equal(t, msg.ThreadID, receipt.ThreadID)
	require.Equal(t, msg.DomainID, receipt.DomainID)
	require.Equal(t, statusViaBot, receipt.Via)
}

func TestInsertSentStatuses_BotSenderNotPromoted(t *testing.T) {
	var (
		bot   = uuid.New()
		human = uuid.New()
	)

	statusStore := &recordingMessageStatusStore{}
	uow := fakeUnitOfWork{messageStatusStore: statusStore}

	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		DomainID: 1,
		From:     shared.Peer{ID: bot},
		To: []*model.ThreadDialog{
			{ContactID: bot, IsBot: true},
			{ContactID: human},
		},
	}

	svc := &MessageService{}
	require.NoError(t, svc.insertSentStatuses(context.Background(), uow, msg))

	// The bot is the sender here: no SENT row and no delivered promotion.
	require.Equal(t, []uuid.UUID{human}, statusStore.insertedRecipients)
	require.Empty(t, statusStore.deliveredReceipts)
}

func TestInsertSentStatuses_NoBots_NoDeliveredCall(t *testing.T) {
	statusStore := &recordingMessageStatusStore{}
	uow := fakeUnitOfWork{messageStatusStore: statusStore}

	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		DomainID: 1,
		From:     shared.Peer{ID: uuid.New()},
		To: []*model.ThreadDialog{
			{ContactID: uuid.New()},
		},
	}

	svc := &MessageService{}
	require.NoError(t, svc.insertSentStatuses(context.Background(), uow, msg))
	require.Empty(t, statusStore.deliveredReceipts)
}

func TestInsertSentStatuses_SendAsOverridesSender(t *testing.T) {
	var (
		origin    = uuid.New()
		sendAs    = uuid.New()
		recipient = uuid.New()
	)

	statusStore := &recordingMessageStatusStore{}
	uow := fakeUnitOfWork{messageStatusStore: statusStore}

	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		DomainID: 1,
		From:     shared.Peer{ID: origin},
		SendAs:   &sendAs,
		To: []*model.ThreadDialog{
			{ContactID: sendAs},
			{ContactID: origin},
			{ContactID: recipient},
		},
	}

	svc := &MessageService{}
	require.NoError(t, svc.insertSentStatuses(context.Background(), uow, msg))

	// The effective sender (send_as) is excluded; the origin operator still
	// receives the message in the thread, so it stays a recipient.
	require.Equal(t, []uuid.UUID{origin, recipient}, statusStore.insertedRecipients)
}
