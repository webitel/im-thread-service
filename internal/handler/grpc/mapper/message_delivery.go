package mapper

import (
	"time"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func MapToMessageDelivery(pb *impb.UpdateMessageDeliveryRequest) *model.MessageDelivery {
	if pb == nil {
		return nil
	}

	var at time.Time
	if ms := pb.GetAt(); ms > 0 {
		at = time.UnixMilli(ms).UTC()
	}

	return &model.MessageDelivery{
		GateID:     pb.GetGateId(),
		ExternalID: pb.GetExternalMessageId(),
		Status:     mapToDeliveryStatus(pb.GetStatus()),
		Reason:     pb.GetReason(),
		At:         at,
		DomainID:   pb.GetDomainId(),
	}
}

func mapToDeliveryStatus(pb impb.MessageDeliveryStatus) model.MessageDeliveryStatus {
	switch pb {
	case impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_SENT:
		return model.MessageDeliveryStatusSent
	case impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_DELIVERED:
		return model.MessageDeliveryStatusDelivered
	case impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_READ:
		return model.MessageDeliveryStatusRead
	case impb.MessageDeliveryStatus_MESSAGE_DELIVERY_STATUS_FAILED:
		return model.MessageDeliveryStatusFailed
	default:
		return model.MessageDeliveryStatusUnspecified
	}
}
