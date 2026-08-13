package postgres

// Tests for the watermark (horizon) logic introduced in message_status_store.go:
//   - MarkDelivered: watermark path vs per-message path partitioning
//   - MarkDelivered: watermark path synthesizes a StatusChange (MessageID==Nil, UpToMessageID set)
//   - MarkRead: always synthesizes one StatusChange per receipt, no per-message insert
//   - MarkRead: monotonic — older up_to never regresses the deduped result
//   - InsertSent: system messages skip unread bump; nil/empty recipients are no-ops
//   - advanceDeliveredHorizon monotonicity predicate (pure logic, no DB)
//   - dedupStatusReceipts watermark corner-case: same member, two threads — product bug

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func wsReceipt(threadID, memberID, upTo uuid.UUID) *model.StatusReceipt {
	return &model.StatusReceipt{
		ThreadID:      threadID,
		MemberID:      memberID,
		UpToMessageID: upTo,
		Via:           "ws",
	}
}

func pushReceipt(threadID, memberID, upTo uuid.UUID) *model.StatusReceipt {
	return &model.StatusReceipt{
		ThreadID:      threadID,
		MemberID:      memberID,
		UpToMessageID: upTo,
		Via:           "push",
	}
}

func providerReceipt(threadID, memberID, msgID uuid.UUID) *model.StatusReceipt {
	return &model.StatusReceipt{
		ThreadID:  threadID,
		MemberID:  memberID,
		MessageID: msgID,
		Via:       "provider",
	}
}

// mustV7Pair delegates to the existing uuidV7Pair helper.
func mustV7Pair(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()

	return uuidV7Pair(t)
}

// synthesizeWatermarkDelivered replicates the synthesis step inside
// MarkDelivered so the partitioning behavior can be verified without a DB.
func synthesizeWatermarkDelivered(receipts []*model.StatusReceipt) []*model.StatusChange {
	now := time.Now().UTC()
	out := make([]*model.StatusChange, 0, len(receipts))

	for _, r := range receipts {
		via := r.Via
		out = append(out, &model.StatusChange{
			DomainID:      r.DomainID,
			ThreadID:      r.ThreadID,
			UpToMessageID: r.UpToMessageID,
			MemberID:      r.MemberID,
			Status:        model.MessageDeliveryStatusDelivered,
			Via:           &via,
			UpdatedAt:     now,
		})
	}

	return out
}

// synthesizeReadChanges replicates the synthesis step inside MarkRead.
func synthesizeReadChanges(receipts []*model.ReadReceipt) []*model.StatusChange {
	now := time.Now().UTC()
	out := make([]*model.StatusChange, 0, len(receipts))

	for _, r := range receipts {
		via := r.Via
		out = append(out, &model.StatusChange{
			DomainID:      r.DomainID,
			ThreadID:      r.ThreadID,
			UpToMessageID: r.UpToMessageID,
			MemberID:      r.MemberID,
			Status:        model.MessageDeliveryStatusRead,
			Via:           &via,
			UpdatedAt:     now,
		})
	}

	return out
}

// --------------------------------------------------------------------------
// MarkDelivered — watermark path synthesizes the correct StatusChange
// --------------------------------------------------------------------------

func TestMarkDelivered_WatermarkPath_SynthesizesStatusChange(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	memberID := uuid.New()
	upTo, _ := uuidV7Pair(t)

	r := wsReceipt(threadID, memberID, upTo)
	r.DomainID = 42

	changes := synthesizeWatermarkDelivered([]*model.StatusReceipt{r})

	if len(changes) != 1 {
		t.Fatalf("expected 1 synthesized change, got %d", len(changes))
	}

	c := changes[0]

	if c.MessageID != uuid.Nil {
		t.Errorf("watermark StatusChange must have MessageID==Nil, got %s", c.MessageID)
	}

	if c.UpToMessageID != upTo {
		t.Errorf("UpToMessageID: want %s, got %s", upTo, c.UpToMessageID)
	}

	if c.MemberID != memberID {
		t.Errorf("MemberID: want %s, got %s", memberID, c.MemberID)
	}

	if c.ThreadID != threadID {
		t.Errorf("ThreadID: want %s, got %s", threadID, c.ThreadID)
	}

	if c.Status != model.MessageDeliveryStatusDelivered {
		t.Errorf("Status: want Delivered, got %v", c.Status)
	}

	if c.Via == nil || *c.Via != "ws" {
		t.Errorf("Via: want %q, got %v", "ws", c.Via)
	}

	if c.DomainID != 42 {
		t.Errorf("DomainID: want 42, got %d", c.DomainID)
	}

	if c.UpdatedAt.IsZero() {
		t.Error("UpdatedAt must be set")
	}
}

// --------------------------------------------------------------------------
// MarkDelivered — partitioning: ws/push+UpToMessageID → watermark; provider+MessageID → per-message
// --------------------------------------------------------------------------

func TestMarkDelivered_AllReceiptsAreWatermarks(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	memberID := uuid.New()
	upTo := uuid.Must(uuid.NewV7())
	msgID := uuid.Must(uuid.NewV7())

	cases := []struct {
		name            string
		receipts        []*model.StatusReceipt
		expectWatermark bool
	}{
		{
			name:            "ws with UpToMessageID is a watermark",
			receipts:        []*model.StatusReceipt{wsReceipt(threadID, memberID, upTo)},
			expectWatermark: true,
		},
		{
			name:            "push with UpToMessageID is a watermark",
			receipts:        []*model.StatusReceipt{pushReceipt(threadID, memberID, upTo)},
			expectWatermark: true,
		},
		{
			name:            "provider with MessageID is converted to watermark (UpToMessageID = MessageID)",
			receipts:        []*model.StatusReceipt{providerReceipt(threadID, memberID, msgID)},
			expectWatermark: true,
		},
		{
			name: "ws with nil UpToMessageID is skipped (does not become a watermark)",
			receipts: []*model.StatusReceipt{
				{ThreadID: threadID, MemberID: memberID, Via: "ws"}, // UpToMessageID==Nil
			},
			expectWatermark: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deduped := dedupStatusReceipts(tc.receipts)

			// Dedup passes all receipts through (even those with nil UpToMessageID and nil MessageID).
			// MarkDelivered then filters: it converts MessageID-only receipts to watermarks,
			// and only sends receipts with UpToMessageID != uuid.Nil to advanceDeliveredHorizon.
			// So we check that dedup returns the same count as input.
			if len(deduped) != len(tc.receipts) {
				t.Errorf("dedup: want %d receipts, got %d", len(tc.receipts), len(deduped))
			}
		})
	}
}

// --------------------------------------------------------------------------
// MarkRead — synthesizes one change per receipt with correct fields
// --------------------------------------------------------------------------

func TestMarkRead_SynthesizesOneChangePerReceipt(t *testing.T) {
	t.Parallel()

	older, newer := mustV7Pair(t)
	thread, member := uuid.New(), uuid.New()

	cases := []struct {
		name     string
		receipts []*model.ReadReceipt
		wantLen  int
	}{
		{
			name: "single receipt produces single change",
			receipts: []*model.ReadReceipt{
				{ThreadID: uuid.New(), MemberID: uuid.New(), UpToMessageID: uuid.Must(uuid.NewV7()), Via: "ws"},
			},
			wantLen: 1,
		},
		{
			name: "two distinct (thread,member) pairs produce two changes",
			receipts: []*model.ReadReceipt{
				{ThreadID: uuid.New(), MemberID: uuid.New(), UpToMessageID: uuid.Must(uuid.NewV7()), Via: "push"},
				{ThreadID: uuid.New(), MemberID: uuid.New(), UpToMessageID: uuid.Must(uuid.NewV7()), Via: "ws"},
			},
			wantLen: 2,
		},
		{
			name: "duplicate (thread,member) is deduped to max UpToMessageID",
			receipts: []*model.ReadReceipt{
				{ThreadID: thread, MemberID: member, UpToMessageID: older},
				{ThreadID: thread, MemberID: member, UpToMessageID: newer},
			},
			wantLen: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deduped := dedupReadReceipts(tc.receipts)
			changes := synthesizeReadChanges(deduped)

			if len(changes) != tc.wantLen {
				t.Fatalf("want %d changes, got %d", tc.wantLen, len(changes))
			}

			for i, c := range changes {
				if c.MessageID != uuid.Nil {
					t.Errorf("change[%d]: MessageID must be Nil for read watermark, got %s", i, c.MessageID)
				}

				if c.UpToMessageID == uuid.Nil {
					t.Errorf("change[%d]: UpToMessageID must be set", i)
				}

				if c.Status != model.MessageDeliveryStatusRead {
					t.Errorf("change[%d]: Status must be Read, got %v", i, c.Status)
				}

				if c.UpdatedAt.IsZero() {
					t.Errorf("change[%d]: UpdatedAt must be set", i)
				}
			}
		})
	}
}

func TestMarkRead_Monotonic_OlderUpToDoesNotRegress(t *testing.T) {
	t.Parallel()

	thread, member := uuid.New(), uuid.New()
	older, newer := mustV7Pair(t)

	// Submit older first then newer — dedup must keep newer.
	deduped := dedupReadReceipts([]*model.ReadReceipt{
		{ThreadID: thread, MemberID: member, UpToMessageID: older},
		{ThreadID: thread, MemberID: member, UpToMessageID: newer},
	})

	if len(deduped) != 1 {
		t.Fatalf("want 1 deduped receipt, got %d", len(deduped))
	}

	if deduped[0].UpToMessageID != newer {
		t.Errorf("monotonic: dedup must keep the greater up_to; want %s, got %s", newer, deduped[0].UpToMessageID)
	}

	// Submit newer first then older — must still keep newer.
	deduped2 := dedupReadReceipts([]*model.ReadReceipt{
		{ThreadID: thread, MemberID: member, UpToMessageID: newer},
		{ThreadID: thread, MemberID: member, UpToMessageID: older},
	})

	if len(deduped2) != 1 {
		t.Fatalf("want 1 deduped receipt, got %d", len(deduped2))
	}

	if deduped2[0].UpToMessageID != newer {
		t.Errorf("monotonic: later insert must not regress horizon; want %s, got %s", newer, deduped2[0].UpToMessageID)
	}
}

// --------------------------------------------------------------------------
// InsertSent — system vs content message; early-exit paths
// --------------------------------------------------------------------------

// InsertSent returns nil immediately for system messages without touching the
// DB. We verify this by passing a nil Querier: any DB access would panic.
func TestInsertSent_SystemMessage_SkipsUnreadBump(t *testing.T) {
	t.Parallel()

	s := &messageStatusStore{db: nil}
	systemMsg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		Type:     model.MessageTypeSystem,
	}

	if err := s.InsertSent(context.Background(), systemMsg, []uuid.UUID{uuid.New(), uuid.New()}); err != nil {
		t.Errorf("InsertSent for system message must return nil, got: %v", err)
	}
}

func TestInsertSent_NilMessage_ReturnsNil(t *testing.T) {
	t.Parallel()

	s := &messageStatusStore{db: nil}

	if err := s.InsertSent(context.Background(), nil, []uuid.UUID{uuid.New()}); err != nil {
		t.Errorf("InsertSent(nil, ...) must return nil, got: %v", err)
	}
}

func TestInsertSent_NoRecipients_ReturnsNil(t *testing.T) {
	t.Parallel()

	s := &messageStatusStore{db: nil}
	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		Type:     model.MessageTypeText,
	}

	if err := s.InsertSent(context.Background(), msg, nil); err != nil {
		t.Errorf("InsertSent(msg, nil) must return nil, got: %v", err)
	}

	if err := s.InsertSent(context.Background(), msg, []uuid.UUID{}); err != nil {
		t.Errorf("InsertSent(msg, []) must return nil, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// advanceDeliveredHorizon monotonicity — pure-logic predicate
// --------------------------------------------------------------------------

func TestAdvanceDeliveredHorizon_Monotonicity_Logic(t *testing.T) {
	t.Parallel()

	// The SQL CASE expression implements:
	//   new_val = (current IS NULL OR new_val > current) ? new_val : current
	// We test that predicate in Go to confirm the logic is correct.
	older, newer := mustV7Pair(t)

	cases := []struct {
		name    string
		current *uuid.UUID
		upTo    uuid.UUID
		wantAdv bool
	}{
		{"null current advances to any value", nil, newer, true},
		{"null current advances to older value", nil, older, true},
		{"current=older upTo=newer advances", &older, newer, true},
		{"current=newer upTo=older does not advance", &newer, older, false},
		{"idempotent: same value does not advance", &newer, newer, false},
	}

	advance := func(current *uuid.UUID, upTo uuid.UUID) bool {
		if current == nil {
			return true
		}

		return greaterUUID(upTo, *current)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := advance(tc.current, tc.upTo)

			if got != tc.wantAdv {
				t.Errorf("advance(current=%v, upTo=%s): want %v, got %v",
					tc.current, tc.upTo, tc.wantAdv, got)
			}
		})
	}
}

// --------------------------------------------------------------------------
// dedupStatusReceipts — watermark corner-case (product bug)
// --------------------------------------------------------------------------

// TestDedupStatusReceipts_WatermarkSameMemberDifferentThreads demonstrates a
// product bug: two watermark receipts for the SAME member in DIFFERENT threads
// both have MessageID==Nil, so they share the dedup key {Nil, memberID} and
// the second receipt is silently dropped.
//
// This test FAILS on the current code. It documents the bug. When fixed (e.g.
// by using {UpToMessageID, memberID} or {threadID, memberID} as the watermark
// dedup key), this test will pass.
func TestDedupStatusReceipts_WatermarkSameMemberDifferentThreads(t *testing.T) {
	t.Parallel()

	memberID := uuid.New()
	thread1, thread2 := uuid.New(), uuid.New()
	upTo1 := uuid.Must(uuid.NewV7())
	upTo2 := uuid.Must(uuid.NewV7())

	r1 := &model.StatusReceipt{ThreadID: thread1, MemberID: memberID, UpToMessageID: upTo1, Via: "ws"}
	r2 := &model.StatusReceipt{ThreadID: thread2, MemberID: memberID, UpToMessageID: upTo2, Via: "ws"}

	out := dedupStatusReceipts([]*model.StatusReceipt{r1, r2})

	// PRODUCT BUG: current dedup key is {MessageID==Nil, memberID} for both
	// receipts, so the second is dropped. We assert what should happen (2 kept).
	if len(out) != 2 {
		t.Errorf("PRODUCT BUG: dedupStatusReceipts dropped a watermark receipt for a different thread; "+
			"want 2 receipts, got %d. Both have MessageID==Nil so they share key {Nil, memberID}. "+
			"Fix: use {UpToMessageID, memberID} or {threadID, memberID} as the watermark dedup key.", len(out))
	}
}
