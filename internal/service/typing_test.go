package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type publishedTyping struct {
	topic   string
	payload []byte
}

type fakeTypingBus struct {
	published []publishedTyping
	err       error
}

func (f *fakeTypingBus) Publish(topic string, msgs ...*message.Message) error {
	if f.err != nil {
		return f.err
	}

	for _, m := range msgs {
		f.published = append(f.published, publishedTyping{topic: topic, payload: m.Payload})
	}

	return nil
}

type fakeRateLimiter struct {
	allow bool
	calls int
}

func (f *fakeRateLimiter) Allow(context.Context, string) bool {
	f.calls++

	return f.allow
}

type fakeProvidersAdapter struct {
	typingCalls []*model.TypingDispatch
}

func (f *fakeProvidersAdapter) SendMessage(context.Context, *model.Message) error { return nil }

func (f *fakeProvidersAdapter) SendTyping(_ context.Context, req *model.TypingDispatch) error {
	f.typingCalls = append(f.typingCalls, req)

	return nil
}

func defaultTypingCfg() config.TypingConfig {
	cfg := config.TypingConfig{Enabled: true}
	cfg.RateLimitWindow = 3 * time.Second
	cfg.DefaultTimeout = 6 * time.Second
	cfg.MaxTimeout = 30 * time.Second
	cfg.MaxPreviewBytes = 1024

	return cfg
}

func newTypingService(cfg config.TypingConfig, bus *fakeTypingBus, rl RateLimiter, members []*model.ThreadDialog) *MessageService {
	return &MessageService{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow:         fakeUnitOfWork{threadDialogStore: &fakeThreadDialogStore{quickViewResult: members}},
		typingBus:   bus,
		rateLimiter: rl,
		typingCfg:   cfg,
	}
}

func decodeTyping(t *testing.T, bus *fakeTypingBus) event.Typing {
	t.Helper()
	require.Len(t, bus.published, 1)

	var ev event.Typing
	require.NoError(t, json.Unmarshal(bus.published[0].payload, &ev))

	return ev
}

func TestSendTyping_FeatureDisabled_NoPublish(t *testing.T) {
	cfg := defaultTypingCfg()
	cfg.Enabled = false

	bus := &fakeTypingBus{}
	rl := &fakeRateLimiter{allow: true}
	svc := newTypingService(cfg, bus, rl, nil)

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:     shared.Peer{ID: uuid.New()},
		ThreadID: uuid.New(),
	})

	require.NoError(t, err)
	require.Empty(t, bus.published)
	require.Zero(t, rl.calls, "disabled feature must short-circuit before rate limiting")
}

func TestSendTyping_RateLimited_Dropped(t *testing.T) {
	bus := &fakeTypingBus{}
	rl := &fakeRateLimiter{allow: false}
	svc := newTypingService(defaultTypingCfg(), bus, rl, nil)

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:     shared.Peer{ID: uuid.New()},
		ThreadID: uuid.New(),
	})

	require.NoError(t, err)
	require.Empty(t, bus.published)
	require.Equal(t, 1, rl.calls)
}

func TestSendTyping_NonMember_Rejected(t *testing.T) {
	threadID := uuid.New()
	member := &model.ThreadDialog{ContactID: uuid.New(), ThreadID: threadID}

	bus := &fakeTypingBus{}
	svc := newTypingService(defaultTypingCfg(), bus, &fakeRateLimiter{allow: true}, []*model.ThreadDialog{member})

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:     shared.Peer{ID: uuid.New()}, // not a member
		ThreadID: threadID,
	})

	require.Error(t, err)
	require.Empty(t, bus.published)
}

func TestSendTyping_PlainTyping_PublishesToTopicWithoutPreview(t *testing.T) {
	threadID := uuid.New()
	sender := uuid.New()
	members := []*model.ThreadDialog{
		{ContactID: sender, ThreadID: threadID},
		{ContactID: uuid.New(), ThreadID: threadID},
	}

	cfg := defaultTypingCfg() // PreviewEnabled defaults to false
	bus := &fakeTypingBus{}
	svc := newTypingService(cfg, bus, &fakeRateLimiter{allow: true}, members)

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:        shared.Peer{ID: sender},
		ThreadID:    threadID,
		PreviewText: "should be ignored while preview is off",
	})
	require.NoError(t, err)

	require.Equal(t, event.TypingTopic(threadID), bus.published[0].topic)

	ev := decodeTyping(t, bus)
	require.Equal(t, sender, ev.MemberID)
	require.Equal(t, threadID, ev.ThreadID)
	require.Equal(t, int32(6000), ev.TimeoutMS)
	require.Empty(t, ev.PreviewText)
	require.Empty(t, ev.PreviewVisibleTo)
}

func TestSendTyping_PreviewEnabled_OnlyAgentRecipients(t *testing.T) {
	threadID := uuid.New()
	client := uuid.New()
	operator := uuid.New()
	botID := uuid.New()
	externalID := uuid.New()
	via := "telegram"

	members := []*model.ThreadDialog{
		{ContactID: client, ThreadID: threadID},                // sender (internal)
		{ContactID: operator, ThreadID: threadID},              // agent -> allowed
		{ContactID: botID, ThreadID: threadID, IsBot: true},    // bot -> excluded
		{ContactID: externalID, ThreadID: threadID, Via: &via}, // external peer -> excluded
	}

	cfg := defaultTypingCfg()
	cfg.PreviewEnabled = true

	bus := &fakeTypingBus{}
	svc := newTypingService(cfg, bus, &fakeRateLimiter{allow: true}, members)

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:        shared.Peer{ID: client},
		ThreadID:    threadID,
		PreviewText: "hello operator",
	})
	require.NoError(t, err)

	ev := decodeTyping(t, bus)
	require.Equal(t, "hello operator", ev.PreviewText)
	require.Equal(t, []uuid.UUID{operator}, ev.PreviewVisibleTo)
}

func TestSendTyping_PreviewEnabled_NoAgentRecipient_NoPreview(t *testing.T) {
	threadID := uuid.New()
	client := uuid.New()
	botID := uuid.New()

	members := []*model.ThreadDialog{
		{ContactID: client, ThreadID: threadID},
		{ContactID: botID, ThreadID: threadID, IsBot: true},
	}

	cfg := defaultTypingCfg()
	cfg.PreviewEnabled = true

	bus := &fakeTypingBus{}
	svc := newTypingService(cfg, bus, &fakeRateLimiter{allow: true}, members)

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:        shared.Peer{ID: client},
		ThreadID:    threadID,
		PreviewText: "no one to see this",
	})
	require.NoError(t, err)

	ev := decodeTyping(t, bus)
	require.Empty(t, ev.PreviewText)
	require.Empty(t, ev.PreviewVisibleTo)
}

func TestSendTyping_PreviewTruncatedOnRuneBoundary(t *testing.T) {
	threadID := uuid.New()
	client := uuid.New()
	operator := uuid.New()
	members := []*model.ThreadDialog{
		{ContactID: client, ThreadID: threadID},
		{ContactID: operator, ThreadID: threadID},
	}

	cfg := defaultTypingCfg()
	cfg.PreviewEnabled = true
	cfg.MaxPreviewBytes = 5 // fits one 3-byte rune + would split the second

	bus := &fakeTypingBus{}
	svc := newTypingService(cfg, bus, &fakeRateLimiter{allow: true}, members)

	// "日" is 3 bytes; two of them = 6 bytes > 5, so only the first must remain.
	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:        shared.Peer{ID: client},
		ThreadID:    threadID,
		PreviewText: "日日",
	})
	require.NoError(t, err)

	ev := decodeTyping(t, bus)
	require.Equal(t, "日", ev.PreviewText)
	require.True(t, len(ev.PreviewText) <= cfg.MaxPreviewBytes)
}

func TestSendTyping_PopulatesInternalRecipients(t *testing.T) {
	threadID := uuid.New()
	client := uuid.New()
	operator := uuid.New()
	botID := uuid.New()
	via := "telegram"
	externalID := uuid.New()

	members := []*model.ThreadDialog{
		{ContactID: client, ThreadID: threadID},
		{ContactID: operator, ThreadID: threadID},
		{ContactID: botID, ThreadID: threadID, IsBot: true},
		{ContactID: externalID, ThreadID: threadID, Via: &via},
	}

	bus := &fakeTypingBus{}
	svc := newTypingService(defaultTypingCfg(), bus, &fakeRateLimiter{allow: true}, members)

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:     shared.Peer{ID: client},
		ThreadID: threadID,
	})
	require.NoError(t, err)

	ev := decodeTyping(t, bus)
	// Only the internal, non-bot, non-sender participant is a recipient.
	require.Equal(t, []uuid.UUID{operator}, ev.To)
}

func TestSendTyping_DispatchesExternalPeersToProviders(t *testing.T) {
	threadID := uuid.New()
	client := uuid.New()
	operator := uuid.New()
	via := "telegram"
	externalID := uuid.New()

	members := []*model.ThreadDialog{
		{ContactID: client, ThreadID: threadID},
		{ContactID: operator, ThreadID: threadID},
		{ContactID: externalID, ThreadID: threadID, Via: &via},
	}

	bus := &fakeTypingBus{}
	providers := &fakeProvidersAdapter{}
	svc := newTypingService(defaultTypingCfg(), bus, &fakeRateLimiter{allow: true}, members)
	svc.providersAdapter = providers

	_, err := svc.SendTyping(context.Background(), &dto.SendTypingRequest{
		From:     shared.Peer{ID: client},
		ThreadID: threadID,
		DomainID: 1,
	})
	require.NoError(t, err)

	require.Len(t, providers.typingCalls, 1)
	call := providers.typingCalls[0]
	require.True(t, call.TypingOn)
	require.Equal(t, threadID, call.ThreadID)
	require.Len(t, call.Peers, 1)
	require.Equal(t, externalID, call.Peers[0].ContactID)
	require.Equal(t, via, call.Peers[0].Via)
}

func TestResolveTypingTimeout(t *testing.T) {
	cfg := defaultTypingCfg()
	svc := &MessageService{typingCfg: cfg}

	cases := []struct {
		name      string
		requested int32
		want      int32
	}{
		{"unset uses default", 0, 6000},
		{"custom within bounds", 12000, 12000},
		{"above max is clamped", 60000, 30000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, svc.resolveTypingTimeout(tc.requested))
		})
	}
}

func TestTruncateUTF8(t *testing.T) {
	require.Equal(t, "abc", truncateUTF8("abc", 10))
	require.Equal(t, "ab", truncateUTF8("abcdef", 2))
	require.Equal(t, "", truncateUTF8("日", 2))   // would split the 3-byte rune
	require.Equal(t, "日", truncateUTF8("日本", 3)) // keep exactly one rune
	require.Equal(t, strings.Repeat("x", 4), truncateUTF8(strings.Repeat("x", 8), 4))
}
