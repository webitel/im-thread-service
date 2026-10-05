package mapper

import (
	"testing"
	"time"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
)

func TestMapToDeliveryReceipts(t *testing.T) {
	threadID, messageID, memberID := uuid.New(), uuid.New(), uuid.New()

	in := []*impb.DeliveryReceipt{
		nil, // nil entries are skipped
		{
			ThreadId:    threadID.String(),
			MessageId:   messageID.String(),
			MemberId:    memberID.String(),
			DeliveredAt: 1_770_000_000_000,
			Via:         "ws",
			DomainId:    3,
		},
	}

	out, err := MapToDeliveryReceipts(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(out))
	}

	got := out[0]

	if got.ThreadID != threadID || got.MessageID != messageID || got.MemberID != memberID {
		t.Errorf("ids mismatch: %+v", got)
	}

	if !got.At.Equal(time.UnixMilli(1_770_000_000_000)) || got.Via != "ws" || got.DomainID != 3 {
		t.Errorf("attrs mismatch: %+v", got)
	}
}

func TestMapToDeliveryReceipts_InvalidUUID(t *testing.T) {
	_, err := MapToDeliveryReceipts([]*impb.DeliveryReceipt{
		{ThreadId: "nope", MessageId: uuid.New().String(), MemberId: uuid.New().String()},
	})
	if err == nil {
		t.Fatal("expected an invalid-argument error for a broken thread_id")
	}
}

func TestMapToDeliveryReceipts_ZeroTimestampMeansUnset(t *testing.T) {
	out, err := MapToDeliveryReceipts([]*impb.DeliveryReceipt{
		{
			ThreadId:  uuid.New().String(),
			MessageId: uuid.New().String(),
			MemberId:  uuid.New().String(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out[0].At.IsZero() {
		t.Errorf("zero delivered_at must map to the zero time, got %v", out[0].At)
	}
}

func TestMapToReadReceipts(t *testing.T) {
	threadID, memberID, upTo := uuid.New(), uuid.New(), uuid.New()

	out, err := MapToReadReceipts([]*impb.ReadReceipt{
		{
			ThreadId:      threadID.String(),
			MemberId:      memberID.String(),
			UpToMessageId: upTo.String(),
			ReadAt:        1_770_000_111_000,
			Via:           "provider",
			DomainId:      1,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out[0]

	if got.ThreadID != threadID || got.MemberID != memberID || got.UpToMessageID != upTo {
		t.Errorf("ids mismatch: %+v", got)
	}

	if !got.At.Equal(time.UnixMilli(1_770_000_111_000)) || got.Via != "provider" {
		t.Errorf("attrs mismatch: %+v", got)
	}
}

func TestMapToReadReceipts_InvalidUpToMessageID(t *testing.T) {
	_, err := MapToReadReceipts([]*impb.ReadReceipt{
		{ThreadId: uuid.New().String(), MemberId: uuid.New().String(), UpToMessageId: "broken"},
	})
	if err == nil {
		t.Fatal("expected an invalid-argument error for a broken up_to_message_id")
	}
}

func TestMapToFailureReceipts(t *testing.T) {
	out, err := MapToFailureReceipts([]*impb.FailureReceipt{
		{
			ThreadId:     uuid.New().String(),
			MessageId:    uuid.New().String(),
			MemberId:     uuid.New().String(),
			FailedAt:     1_770_000_222_000,
			Via:          "provider",
			ErrorCode:    "131047",
			ErrorMessage: "re-engagement window expired",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out[0]

	if got.Error["code"] != "131047" || got.Error["message"] != "re-engagement window expired" {
		t.Errorf("error details mismatch: %+v", got.Error)
	}
}

func TestMapToFailureReceipts_NoErrorDetails(t *testing.T) {
	out, err := MapToFailureReceipts([]*impb.FailureReceipt{
		{
			ThreadId:  uuid.New().String(),
			MessageId: uuid.New().String(),
			MemberId:  uuid.New().String(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out[0].Error != nil {
		t.Errorf("expected nil error details, got %+v", out[0].Error)
	}
}

// A socket ACK names only the delivered-up-to point; it must not be rejected for a missing message_id.
func TestMapToDeliveryReceipts_AckWithoutMessageID(t *testing.T) {
	thread, member, upTo := uuid.New(), uuid.New(), uuid.New()

	for name, r := range map[string]*impb.DeliveryReceipt{
		"up_to_message_id": {ThreadId: thread.String(), MemberId: member.String(), UpToMessageId: upTo.String()},
		"up_to_seq":        {ThreadId: thread.String(), MemberId: member.String(), UpToSeq: 121},
	} {
		got, err := MapToDeliveryReceipts([]*impb.DeliveryReceipt{r})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}

		if got[0].MessageID != uuid.Nil || got[0].MemberID != member {
			t.Errorf("%s: receipt = %+v", name, got[0])
		}
	}
}

func TestMapToDeliveryReceipts_NoPointRejected(t *testing.T) {
	_, err := MapToDeliveryReceipts([]*impb.DeliveryReceipt{{ThreadId: uuid.New().String(), MemberId: uuid.New().String()}})
	if err == nil {
		t.Fatal("expected an error for a receipt without message_id, up_to_message_id or up_to_seq")
	}

	_, err = MapToDeliveryReceipts([]*impb.DeliveryReceipt{{ThreadId: uuid.New().String(), MemberId: uuid.New().String(), MessageId: "broken"}})
	if err == nil {
		t.Fatal("expected an error for a broken message_id")
	}
}

// A read by seq alone is valid; a read naming nothing is not.
func TestMapToReadReceipts_BySeq(t *testing.T) {
	got, err := MapToReadReceipts([]*impb.ReadReceipt{{ThreadId: uuid.New().String(), MemberId: uuid.New().String(), UpToSeq: 9}})
	if err != nil || got[0].UpToSeq != 9 || got[0].UpToMessageID != uuid.Nil {
		t.Fatalf("got %+v, err %v", got, err)
	}

	if _, err := MapToReadReceipts([]*impb.ReadReceipt{{ThreadId: uuid.New().String(), MemberId: uuid.New().String()}}); err == nil {
		t.Fatal("expected an error for a read without up_to_message_id or up_to_seq")
	}
}
