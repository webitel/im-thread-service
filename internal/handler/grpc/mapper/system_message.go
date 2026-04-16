package mapper

import (
	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func ConvertPbSystemMessageToDomain(in *impb.SendSystemMessageRequest) *model.SystemMessage {
	if in == nil {
		return nil
	}
	return &model.SystemMessage{
		DomainID:       in.GetDomainId(),
		From:           MapPeerFromProto(in.GetFrom()),
		To:             MapPeerFromProto(in.GetTo()),
		Type:           in.GetType(),
		Body:           in.GetBody(),
		Metadata:       in.GetMetadata().AsMap(),
		IdempotencyKey: in.GetSendId(),
	}
}
