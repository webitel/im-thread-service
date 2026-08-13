package mapper

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func MapToDeliveryReceipts(in []*impb.DeliveryReceipt) ([]*model.StatusReceipt, error) {
	out := make([]*model.StatusReceipt, 0, len(in))

	for i, r := range in {
		if r == nil {
			continue
		}

		threadID, err := parseReceiptUUID(r.GetThreadId(), "thread_id", i)
		if err != nil {
			return nil, err
		}

		messageID, err := parseReceiptUUID(r.GetMessageId(), "message_id", i)
		if err != nil {
			return nil, err
		}

		memberID, err := parseReceiptUUID(r.GetMemberId(), "member_id", i)
		if err != nil {
			return nil, err
		}

		upToMessageID, _ := uuid.Parse(r.GetUpToMessageId())

		out = append(out, &model.StatusReceipt{
			DomainID:      r.GetDomainId(),
			ThreadID:      threadID,
			MessageID:     messageID,
			UpToMessageID: upToMessageID,
			UpToSeq:       r.GetUpToSeq(),
			MemberID:      memberID,
			At:            unixMillisToTime(r.GetDeliveredAt()),
			Via:           r.GetVia(),
		})
	}

	return out, nil
}

func MapToReadReceipts(in []*impb.ReadReceipt) ([]*model.ReadReceipt, error) {
	out := make([]*model.ReadReceipt, 0, len(in))

	for i, r := range in {
		if r == nil {
			continue
		}

		threadID, err := parseReceiptUUID(r.GetThreadId(), "thread_id", i)
		if err != nil {
			return nil, err
		}

		memberID, err := parseReceiptUUID(r.GetMemberId(), "member_id", i)
		if err != nil {
			return nil, err
		}

		upToMessageID, err := parseReceiptUUID(r.GetUpToMessageId(), "up_to_message_id", i)
		if err != nil {
			return nil, err
		}

		out = append(out, &model.ReadReceipt{
			DomainID:      r.GetDomainId(),
			ThreadID:      threadID,
			MemberID:      memberID,
			UpToMessageID: upToMessageID,
			UpToSeq:       r.GetUpToSeq(),
			At:            unixMillisToTime(r.GetReadAt()),
			Via:           r.GetVia(),
		})
	}

	return out, nil
}

func MapToFailureReceipts(in []*impb.FailureReceipt) ([]*model.StatusReceipt, error) {
	out := make([]*model.StatusReceipt, 0, len(in))

	for i, r := range in {
		if r == nil {
			continue
		}

		threadID, err := parseReceiptUUID(r.GetThreadId(), "thread_id", i)
		if err != nil {
			return nil, err
		}

		messageID, err := parseReceiptUUID(r.GetMessageId(), "message_id", i)
		if err != nil {
			return nil, err
		}

		memberID, err := parseReceiptUUID(r.GetMemberId(), "member_id", i)
		if err != nil {
			return nil, err
		}

		var errDetails map[string]any
		if r.GetErrorCode() != "" || r.GetErrorMessage() != "" {
			errDetails = map[string]any{
				"code":    r.GetErrorCode(),
				"message": r.GetErrorMessage(),
			}
		}

		out = append(out, &model.StatusReceipt{
			DomainID:  r.GetDomainId(),
			ThreadID:  threadID,
			MessageID: messageID,
			MemberID:  memberID,
			At:        unixMillisToTime(r.GetFailedAt()),
			Via:       r.GetVia(),
			Error:     errDetails,
		})
	}

	return out, nil
}

func parseReceiptUUID(raw, field string, index int) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.InvalidArgument(
			fmt.Sprintf("receipts[%d].%s: invalid uuid", index, field),
			errors.WithCause(err),
			errors.WithID("handler.message_status.receipt"),
		)
	}

	return id, nil
}

func unixMillisToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}

	return time.UnixMilli(ms)
}
