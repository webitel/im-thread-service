package mapper

import (
	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func ConvertPbSystemMessageToDomain(in *impb.SendSystemMessageRequest) *model.Message {
	if in == nil {
		return nil
	}

	sendAs, _ := uuid.Parse(in.GetSendAs())

	return &model.Message{
		DomainID:       in.GetDomainId(),
		From:           MapPeerFromProto(in.GetFrom()),
		SendTo:         MapPeerFromProto(in.GetTo()),
		Type:           model.MessageTypeSystem,
		Body:           in.GetBody(),
		Metadata:       in.GetMetadata().AsMap(),
		IdempotencyKey: in.GetSendId(),
		System: &model.MessageSystem{
			Type:     in.GetType(),
			Metadata: in.GetMetadata().AsMap(),
		},
		SendAs: &sendAs,
	}
}
