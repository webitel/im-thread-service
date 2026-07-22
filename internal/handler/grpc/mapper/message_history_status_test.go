package mapper

import (
	"testing"
	"time"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestMapDeliveryStatus(t *testing.T) {
	if got := mapDeliveryStatus(nil); got != impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_UNSPECIFIED {
		t.Errorf("nil aggregate must map to UNSPECIFIED, got %v", got)
	}

	read := model.MessageDeliveryStatusRead
	if got := mapDeliveryStatus(&read); got != impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_READ {
		t.Errorf("expected READ, got %v", got)
	}
}

func TestMapRecipientStatuses(t *testing.T) {
	member := uuid.New()
	deliveredAt := time.UnixMilli(1_770_000_000_000)
	via := "provider"

	out := mapRecipientStatuses([]*model.MessageRecipientStatus{
		nil, // nil entries are skipped
		{
			MemberID:    member,
			Status:      model.MessageDeliveryStatusFailed,
			DeliveredAt: &deliveredAt,
			Via:         &via,
			Error:       map[string]any{"code": "470", "message": "undeliverable"},
		},
	})

	if len(out) != 1 {
		t.Fatalf("expected 1 status, got %d", len(out))
	}

	got := out[0]

	if got.GetMemberId() != member.String() || got.GetStatus() != impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_FAILED {
		t.Errorf("identity mismatch: %+v", got)
	}

	if got.GetDeliveredAt() != deliveredAt.UnixMilli() {
		t.Errorf("delivered_at = %d, want %d", got.GetDeliveredAt(), deliveredAt.UnixMilli())
	}

	if got.GetReadAt() != 0 || got.GetFailedAt() != 0 {
		t.Errorf("unset timestamps must stay zero: %+v", got)
	}

	if got.GetVia() != via {
		t.Errorf("via = %q, want %q", got.GetVia(), via)
	}

	// The error payload is passed to clients as a JSON string.
	if got.GetError() == "" || got.GetError()[0] != '{' {
		t.Errorf("expected JSON-encoded error, got %q", got.GetError())
	}
}

func TestMapRecipientStatuses_Empty(t *testing.T) {
	if got := mapRecipientStatuses(nil); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}
