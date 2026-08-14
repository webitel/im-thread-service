package service

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/service/guards"
)

// TypingBus publishes ephemeral events straight to the broker, bypassing the
// transactional outbox. The existing RabbitMQ EventPublisher satisfies it.
type TypingBus interface {
	Publish(topic string, messages ...*message.Message) error
}

// RateLimiter enforces a best-effort, TTL-bounded per-key rate limit.
type RateLimiter interface {
	Allow(ctx context.Context, key string) bool
}

// SendTyping publishes an ephemeral "…is typing" indicator (optionally with a
// live draft preview) to the other online participants of a thread.
//
// It is fire-and-forget and deliberately best-effort: it never persists
// anything and returns success even when the event is dropped (feature off,
// rate-limited, or publish failure). Only real input errors (bad request,
// non-member sender) surface to the caller.
func (s *MessageService) SendTyping(ctx context.Context, in *dto.SendTypingRequest) (*dto.SendTypingResponse, error) {
	if err := guards.SendTypingGuard(in); err != nil {
		return nil, errors.InvalidArgument("typing: invalid request", errors.WithCause(err))
	}

	// Feature master switch: accept-and-ignore keeps callers simple.
	if !s.typingCfg.Enabled {
		return &dto.SendTypingResponse{}, nil
	}

	// Throttle per participant per thread; excess events are dropped silently.
	rlKey := in.ThreadID.String() + ":" + in.From.ID.String()
	if s.rateLimiter != nil && !s.rateLimiter.Allow(ctx, rlKey) {
		return &dto.SendTypingResponse{}, nil
	}

	// The sender must be an active participant of the thread.
	members, err := s.uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
		ThreadIDs: []uuid.UUID{in.ThreadID},
		DomainID:  int(in.DomainID),
	})
	if err != nil {
		return nil, errors.New("typing: failed to load thread members", errors.WithCause(err))
	}

	if !isActiveMember(members, in.From.ID) {
		return nil, errors.New("typing: sender is not a participant of the thread")
	}

	// Recipients of the indicator: the thread's internal participants (SDK /
	// widget / operators), never the sender. External peers are handled by the
	// providers path below, offline members are dropped by delivery.
	recipients := internalRecipients(members, in.From.ID)

	ev := event.Typing{
		ThreadID:   in.ThreadID,
		MemberID:   in.From.ID,
		Role:       int32(memberRole(members, in.From.ID)),
		DomainID:   int32(in.DomainID),
		TimeoutMS:  s.resolveTypingTimeout(in.TimeoutMS),
		OccurredAt: time.Now().UTC(),
		To:         recipients,
	}

	// Live Typing Preview: attach the draft only when the feature is on AND the
	// thread has at least one authorized (agent-side) recipient. The allow-list
	// is resolved server-side so the draft never enters the stream for peers who
	// are not permitted to see it.
	if s.typingCfg.PreviewEnabled && len(recipients) > 0 {
		ev.PreviewText = truncateUTF8(in.PreviewText, s.typingCfg.MaxPreviewBytes)
		ev.PreviewVisibleTo = recipients
	}

	s.publishTyping(ctx, ev)

	// Forward native typing to external channels (Telegram/Meta/…) that support
	// it. Best-effort: never fails the RPC.
	s.dispatchExternalTyping(ctx, members, in)

	return &dto.SendTypingResponse{}, nil
}

// dispatchExternalTyping forwards a typing action to the thread's external peers
// via im-providers. Fire-and-forget: errors are swallowed (best-effort).
func (s *MessageService) dispatchExternalTyping(ctx context.Context, members []*model.ThreadDialog, in *dto.SendTypingRequest) {
	if s.providersAdapter == nil {
		return
	}

	external := model.ThreadDialogs(members).ExtractExternalPeers()

	peers := make([]*model.ExternalPeerPair, 0, len(external))
	for _, p := range external {
		if p.ContactID != in.From.ID {
			peers = append(peers, p)
		}
	}

	if len(peers) == 0 {
		return
	}

	if err := s.providersAdapter.SendTyping(ctx, &model.TypingDispatch{
		ThreadID: in.ThreadID,
		DomainID: int32(in.DomainID),
		TypingOn: true,
		Peers:    peers,
	}); err != nil {
		s.logger.WarnContext(ctx, "typing: external provider dispatch failed",
			"thread_id", in.ThreadID, "err", err)
	}
}

// resolveTypingTimeout applies the default when unset and clamps to the max.
func (s *MessageService) resolveTypingTimeout(requestedMS int32) int32 {
	d := s.typingCfg.DefaultTimeout
	if requestedMS > 0 {
		d = time.Duration(requestedMS) * time.Millisecond
	}

	if d > s.typingCfg.MaxTimeout {
		d = s.typingCfg.MaxTimeout
	}

	return int32(d.Milliseconds())
}

// publishTyping serializes and fire-and-forget publishes the event to the
// ephemeral typing topic. Failures are logged (with the draft masked by
// event.Typing.LogValue) and swallowed — a lost typing event breaks nothing.
func (s *MessageService) publishTyping(ctx context.Context, ev event.Typing) {
	payload, err := json.Marshal(ev)
	if err != nil {
		s.logger.WarnContext(ctx, "typing: marshal failed", "event", ev, "err", err)

		return
	}

	msg := message.NewMessage(watermill.NewUUID(), payload)
	if err := s.typingBus.Publish(event.TypingTopic(ev.ThreadID), msg); err != nil {
		s.logger.WarnContext(ctx, "typing: ephemeral publish failed", "event", ev, "err", err)
	}
}

// isActiveMember reports whether id is a live participant of the thread.
func isActiveMember(members []*model.ThreadDialog, id uuid.UUID) bool {
	for _, m := range members {
		if m.ContactID == id && m.DeletedAt == nil {
			return true
		}
	}

	return false
}

// memberRole returns the thread role of the given participant, or the
// unspecified role when the member is not found.
func memberRole(members []*model.ThreadDialog, id uuid.UUID) model.ThreadRole {
	for _, m := range members {
		if m.ContactID == id {
			return m.ThreadRole
		}
	}

	return model.UnspecifiedRole
}

// internalRecipients returns the thread's active, internal (Via == nil),
// non-bot participants other than the sender — the recipients of the typing
// indicator and, currently, the preview allow-list. Isolated so the exact
// operator/supervisor predicate for the preview can be tightened after product
// signoff without touching the publish path.
func internalRecipients(members []*model.ThreadDialog, sender uuid.UUID) []uuid.UUID {
	var out []uuid.UUID

	for _, m := range members {
		if isInternalRecipient(m, sender) {
			out = append(out, m.ContactID)
		}
	}

	return out
}

func isInternalRecipient(m *model.ThreadDialog, sender uuid.UUID) bool {
	return m.DeletedAt == nil && m.ContactID != sender && !m.IsBot && m.Via == nil
}

// truncateUTF8 trims s to at most maxBytes bytes without splitting a rune.
func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	return s[:cut]
}
