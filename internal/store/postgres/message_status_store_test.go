package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// uuidV7Pair returns two UUIDv7 values where the second is created later
// (and therefore byte-wise greater).
func uuidV7Pair(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()

	older, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)

	newer, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}

	return older, newer
}

func TestDedupStatusReceipts(t *testing.T) {
	msg, member := uuid.New(), uuid.New()

	first := &model.StatusReceipt{MessageID: msg, MemberID: member, Via: "ws"}
	duplicate := &model.StatusReceipt{MessageID: msg, MemberID: member, Via: "push"}
	other := &model.StatusReceipt{MessageID: uuid.New(), MemberID: member}

	out := dedupStatusReceipts([]*model.StatusReceipt{first, nil, duplicate, other})

	if len(out) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(out))
	}

	if out[0] != first {
		t.Errorf("the first occurrence must win, got %+v", out[0])
	}

	if out[1] != other {
		t.Errorf("distinct pairs must be kept, got %+v", out[1])
	}
}

func TestDedupReadReceipts_GreatestUpToWins(t *testing.T) {
	thread, member := uuid.New(), uuid.New()
	older, newer := uuidV7Pair(t)

	out := dedupReadReceipts([]*model.ReadReceipt{
		{ThreadID: thread, MemberID: member, UpToMessageID: newer},
		nil,
		{ThreadID: thread, MemberID: member, UpToMessageID: older},
	})

	if len(out) != 1 {
		t.Fatalf("expected receipts of one (thread, member) to collapse, got %d", len(out))
	}

	if out[0].UpToMessageID != newer {
		t.Errorf("the greatest up_to_message_id must win: got %s, want %s", out[0].UpToMessageID, newer)
	}
}

func TestDedupReadReceipts_KeepsDistinctMembersInOrder(t *testing.T) {
	thread := uuid.New()
	m1, m2 := uuid.New(), uuid.New()
	upTo, _ := uuidV7Pair(t)

	r1 := &model.ReadReceipt{ThreadID: thread, MemberID: m1, UpToMessageID: upTo}
	r2 := &model.ReadReceipt{ThreadID: thread, MemberID: m2, UpToMessageID: upTo}

	out := dedupReadReceipts([]*model.ReadReceipt{r1, r2})

	if len(out) != 2 || out[0] != r1 || out[1] != r2 {
		t.Fatalf("expected both receipts in input order, got %+v", out)
	}
}

func TestGreaterUUID(t *testing.T) {
	older, newer := uuidV7Pair(t)

	if !greaterUUID(newer, older) {
		t.Errorf("expected %s > %s", newer, older)
	}

	if greaterUUID(older, newer) {
		t.Errorf("expected %s < %s", older, newer)
	}

	if greaterUUID(older, older) {
		t.Error("a uuid must not be greater than itself")
	}
}

func TestConfirmedAtOrNow(t *testing.T) {
	at := time.UnixMilli(1_770_000_000_000)

	if got := confirmedAtOrNow(at); !got.Equal(at) {
		t.Errorf("non-zero time must pass through, got %v", got)
	}

	before := time.Now().UTC()
	got := confirmedAtOrNow(time.Time{})

	if got.Before(before) || got.After(time.Now().UTC()) {
		t.Errorf("zero time must fall back to now, got %v", got)
	}
}
