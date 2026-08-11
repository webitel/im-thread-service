package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type captureOutboxStore struct {
	published []*capturedMessageEvent
}

type capturedMessageEvent struct {
	messageID    uuid.UUID
	internal     bool
	recipientIDs []uuid.UUID
}

func (c *captureOutboxStore) Publish(ctx context.Context, topic string, e event.Outboxer) error {
	if msgCreated, ok := e.(*event.MessageCreated); ok {
		recipientIDs := make([]uuid.UUID, 0, len(msgCreated.To))
		for _, member := range msgCreated.To {
			recipientIDs = append(recipientIDs, member.ContactID)
		}

		c.published = append(c.published, &capturedMessageEvent{
			messageID:    msgCreated.MessageID,
			internal:     msgCreated.Internal,
			recipientIDs: recipientIDs,
		})
	}

	return nil
}

func (c *captureOutboxStore) Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error) {
	_ = ctx
	_ = opt

	return 0, nil
}

type fakeThreadManagerForInternalNote struct {
	thread *model.Thread
}

func (f *fakeThreadManagerForInternalNote) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	return f.thread, nil
}

func (f *fakeThreadManagerForInternalNote) ReleaseBotControl(ctx context.Context, req *dto.ReleaseBotControlRequest) error {
	return nil
}

func TestSendInternalNote_EventToContainsOnlyWebitelUsers(t *testing.T) {
	operatorID := uuid.New()
	clientID := uuid.New()
	botID := uuid.New()
	via := "telegram"
	threadID := uuid.New()

	// Thread members: operator (internal), client (external via telegram), bot
	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false}, // Internal operator
		{ContactID: clientID, Via: &via, IsBot: false},  // External client
		{ContactID: botID, Via: nil, IsBot: true},       // Bot (excluded)
	}

	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	outbox := &captureOutboxStore{}
	providersAdapter := &fakeProvidersAdapterForInternalNote{}
	svc := &MessageService{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:              fakeUnitOfWork{messageStore: &fakeMessageStore{}, outboxStore: outbox},
		providersAdapter: providersAdapter,
		threader:         fakeThreader,
	}

	in := &dto.SendInternalNoteRequest{
		From:     shared.Peer{ID: operatorID},
		To:       shared.Peer{ID: clientID},
		Body:     "Internal note to operator",
		DomainID: 1,
		SendID:   "send-1",
	}

	resp, err := svc.SendInternalNote(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The event should only contain the operator (Via == nil && !IsBot)
	require.Len(t, outbox.published, 1)
	evt := outbox.published[0]
	require.True(t, evt.internal)
	require.Len(t, evt.recipientIDs, 1)
	require.Equal(t, operatorID, evt.recipientIDs[0])
	// Verify client (Via!=nil) and bot (IsBot) are NOT in recipients
	for _, id := range evt.recipientIDs {
		require.NotEqual(t, clientID, id, "external client should not be in recipients")
		require.NotEqual(t, botID, id, "bot should not be in recipients")
	}
}

func TestSendInternalNote_NoExternalProviderCall(t *testing.T) {
	operatorID := uuid.New()
	clientID := uuid.New()
	via := "telegram"
	threadID := uuid.New()

	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false},
		{ContactID: clientID, Via: &via, IsBot: false},
	}

	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	providersAdapter := &fakeProvidersAdapterForInternalNote{}
	svc := &MessageService{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:              fakeUnitOfWork{messageStore: &fakeMessageStore{}, outboxStore: &captureOutboxStore{}},
		providersAdapter: providersAdapter,
		threader:         fakeThreader,
	}

	in := &dto.SendInternalNoteRequest{
		From:     shared.Peer{ID: operatorID},
		To:       shared.Peer{ID: clientID},
		Body:     "Internal note",
		DomainID: 1,
		SendID:   "send-1",
	}

	_, err := svc.SendInternalNote(context.Background(), in)
	require.NoError(t, err)

	// Verify SendMessage was NOT called on the providers adapter
	require.Equal(t, 0, providersAdapter.sendMessageCalls)
}

func TestSendInternalNote_EmptyBodyRejected(t *testing.T) {
	operatorID := uuid.New()
	clientID := uuid.New()

	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false},
		{ContactID: clientID, Via: nil, IsBot: false},
	}

	threadID := uuid.New()
	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	svc := &MessageService{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:      fakeUnitOfWork{messageStore: &fakeMessageStore{}, outboxStore: &captureOutboxStore{}},
		threader: fakeThreader,
	}

	in := &dto.SendInternalNoteRequest{
		From:     shared.Peer{ID: operatorID},
		To:       shared.Peer{ID: clientID},
		Body:     "   ", // whitespace only
		DomainID: 1,
		SendID:   "send-1",
	}

	_, err := svc.SendInternalNote(context.Background(), in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "body cannot be empty")
}

func TestSendInternalNote_MessageMarkedInternal(t *testing.T) {
	operatorID := uuid.New()
	clientID := uuid.New()
	threadID := uuid.New()

	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false},
		{ContactID: clientID, Via: nil, IsBot: false},
	}

	messageStore := &fakeMessageStore{}
	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	svc := &MessageService{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:      fakeUnitOfWork{messageStore: messageStore, outboxStore: &captureOutboxStore{}},
		threader: fakeThreader,
	}

	in := &dto.SendInternalNoteRequest{
		From:     shared.Peer{ID: operatorID},
		To:       shared.Peer{ID: clientID},
		Body:     "Internal note",
		DomainID: 1,
		SendID:   "send-1",
	}

	_, err := svc.SendInternalNote(context.Background(), in)
	require.NoError(t, err)

	// Verify the saved message has Internal=true
	require.Len(t, messageStore.savedMessages, 1)
	savedMsg := messageStore.savedMessages[0]
	require.True(t, savedMsg.Internal)
}

type fakeProvidersAdapterForInternalNote struct {
	sendMessageCalls int
}

func (f *fakeProvidersAdapterForInternalNote) SendMessage(ctx context.Context, message *model.Message) error {
	f.sendMessageCalls++

	return nil
}

func (f *fakeProvidersAdapterForInternalNote) SendTyping(ctx context.Context, typing *model.TypingDispatch) error {
	return nil
}

func (f *fakeProvidersAdapterForInternalNote) SendReaction(ctx context.Context, reaction *model.Reaction, result *model.ReactionResult) error {
	return nil
}

// TestSendInternalNote_AllBotsThread verifies that when the thread contains ONLY bots
// (no Webitel user), the recipient list in the published event is empty.
func TestSendInternalNote_AllBotsThread(t *testing.T) {
	operatorID := uuid.New()
	bot1 := uuid.New()
	bot2 := uuid.New()
	threadID := uuid.New()

	threadMembers := []*model.ThreadDialog{
		{ContactID: bot1, Via: nil, IsBot: true},
		{ContactID: bot2, Via: nil, IsBot: true},
	}

	outbox := &captureOutboxStore{}
	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	svc := &MessageService{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:      fakeUnitOfWork{messageStore: &fakeMessageStore{}, outboxStore: outbox},
		threader: fakeThreader,
	}

	in := &dto.SendInternalNoteRequest{
		From:     shared.Peer{ID: operatorID},
		To:       shared.Peer{ID: bot1},
		Body:     "note with no Webitel recipients",
		DomainID: 1,
		SendID:   "send-all-bots",
	}

	_, err := svc.SendInternalNote(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, outbox.published, 1)
	evt := outbox.published[0]
	require.True(t, evt.internal)
	require.Empty(t, evt.recipientIDs, "no Webitel users → empty recipient list")
}

// TestSendInternalNote_DeletedMemberExcluded verifies that a soft-deleted Webitel
// member (DeletedAt != nil) is not included in internal note recipients.
func TestSendInternalNote_DeletedMemberExcluded(t *testing.T) {
	operatorID := uuid.New()
	deletedOperatorID := uuid.New()
	clientID := uuid.New()
	via := "telegram"
	threadID := uuid.New()

	ts := time.Now().UTC()
	deletedMember := &model.ThreadDialog{ContactID: deletedOperatorID, Via: nil, IsBot: false, DeletedAt: &ts}

	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false},
		deletedMember,
		{ContactID: clientID, Via: &via, IsBot: false},
	}

	outbox := &captureOutboxStore{}
	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	svc := &MessageService{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:      fakeUnitOfWork{messageStore: &fakeMessageStore{}, outboxStore: outbox},
		threader: fakeThreader,
	}

	in := &dto.SendInternalNoteRequest{
		From:     shared.Peer{ID: operatorID},
		To:       shared.Peer{ID: clientID},
		Body:     "note to active operator only",
		DomainID: 1,
		SendID:   "send-deleted-excluded",
	}

	_, err := svc.SendInternalNote(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, outbox.published, 1)
	evt := outbox.published[0]
	require.True(t, evt.internal)

	for _, id := range evt.recipientIDs {
		require.NotEqual(t, deletedOperatorID, id, "deleted member must not be in recipients")
	}
}

// TestForwardMessages_InternalNotePostedWithInternalFlag verifies that when
// ForwardMessages is called with a non-empty InternalNote, a second SaveMessage
// call is made with Internal=true and only Webitel-user recipients.
func TestForwardMessages_InternalNotePostedWithInternalFlag(t *testing.T) {
	operatorID := uuid.New()
	clientID := uuid.New()
	via := "telegram"
	threadID := uuid.New()
	srcMsgID := uuid.Must(uuid.NewV7())

	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false},
		{ContactID: clientID, Via: &via, IsBot: false},
	}

	srcMsg := &model.Message{
		ID:       srcMsgID,
		SenderID: operatorID,
		ThreadID: threadID,
		Body:     "original message",
		Type:     model.MessageTypeText,
	}

	msgStore := &fakeMessageStore{
		forwardSources: []*model.Message{srcMsg},
	}
	outbox := &captureOutboxStore{}
	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	svc := &MessageService{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:              fakeUnitOfWork{messageStore: msgStore, outboxStore: outbox},
		providersAdapter: &fakeProvidersAdapterForInternalNote{},
		threader:         fakeThreader,
	}

	in := &dto.ForwardMessagesRequest{
		From:         shared.Peer{ID: operatorID},
		To:           shared.Peer{ID: clientID},
		MessageIDs:   []uuid.UUID{srcMsgID},
		DomainID:     1,
		SendID:       "fwd-send-1",
		InternalNote: "operator note on forward",
	}

	_, err := svc.ForwardMessages(context.Background(), in)
	require.NoError(t, err)

	// Expect two saved messages: the forwarded copy + the internal note.
	require.Len(t, msgStore.savedMessages, 2, "expected forwarded copy + internal note")

	var noteMsg *model.Message

	for _, m := range msgStore.savedMessages {
		if m.Internal {
			noteMsg = m
		}
	}

	require.NotNil(t, noteMsg, "internal note message must be saved")
	require.True(t, noteMsg.Internal)
	require.Equal(t, "operator note on forward", noteMsg.Body)

	for _, member := range noteMsg.To {
		require.Nil(t, member.Via, "internal note must only target Webitel users (Via==nil)")
		require.False(t, member.IsBot, "internal note must not target bots")
	}
}

// TestForwardMessages_EmptyInternalNote_NoNoteMessage verifies that when
// InternalNote is empty, ForwardMessages saves exactly one message (the forward
// copy) and does not publish an additional internal-note event.
func TestForwardMessages_EmptyInternalNote_NoNoteMessage(t *testing.T) {
	operatorID := uuid.New()
	clientID := uuid.New()
	via := "telegram"
	threadID := uuid.New()
	srcMsgID := uuid.Must(uuid.NewV7())

	threadMembers := []*model.ThreadDialog{
		{ContactID: operatorID, Via: nil, IsBot: false},
		{ContactID: clientID, Via: &via, IsBot: false},
	}

	srcMsg := &model.Message{
		ID:       srcMsgID,
		SenderID: operatorID,
		ThreadID: threadID,
		Body:     "original message",
		Type:     model.MessageTypeText,
	}

	msgStore := &fakeMessageStore{
		forwardSources: []*model.Message{srcMsg},
	}
	outbox := &captureOutboxStore{}
	fakeThreader := &fakeThreadManagerForInternalNote{thread: &model.Thread{ID: threadID, Members: threadMembers}}
	svc := &MessageService{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:              fakeUnitOfWork{messageStore: msgStore, outboxStore: outbox},
		providersAdapter: &fakeProvidersAdapterForInternalNote{},
		threader:         fakeThreader,
	}

	in := &dto.ForwardMessagesRequest{
		From:         shared.Peer{ID: operatorID},
		To:           shared.Peer{ID: clientID},
		MessageIDs:   []uuid.UUID{srcMsgID},
		DomainID:     1,
		SendID:       "fwd-send-2",
		InternalNote: "", // empty — no extra note
	}

	_, err := svc.ForwardMessages(context.Background(), in)
	require.NoError(t, err)

	require.Len(t, msgStore.savedMessages, 1, "no InternalNote → only the forwarded copy is saved")

	for _, m := range msgStore.savedMessages {
		require.False(t, m.Internal, "forwarded message must not be marked Internal")
	}
}
