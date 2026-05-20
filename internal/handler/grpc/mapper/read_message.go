package mapper

import (
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToReadMessageRequest(pb *impb.ReadMessageRequest) *dto.ReadMessageRequest {
	if pb == nil {
		return nil
	}

	return &dto.ReadMessageRequest{
		MessageID: pb.GetId(),
		ThreadID:  pb.GetThreadId(),
		UserID:    pb.GetUserId(),
		DomainID:  pb.GetDomainId(),
	}
}
