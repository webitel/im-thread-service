package mapper

import (
	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
)

func MapExternalForwardOrigin(in *impb.ForwardOriginInput) *model.ForwardOrigin {
	if in == nil {
		return nil
	}

	kind := model.ForwardOriginKind(in.GetKind())
	if !kind.IsExternal() {
		return nil
	}

	return &model.ForwardOrigin{
		Kind:           kind,
		SenderName:     in.GetSenderName(),
		OriginalSentAt: in.GetOriginalSentAt(),
	}
}

func MapToForwardMessagesRequest(in *impb.ForwardMessagesRequest) *dto.ForwardMessagesRequest {
	if in == nil {
		return nil
	}

	sendAs, _ := uuid.Parse(in.GetSendAs())

	return &dto.ForwardMessagesRequest{
		From:       MapPeerFromProto(in.GetFrom()),
		To:         MapPeerFromProto(in.GetTo()),
		DomainID:   int64(in.GetDomainId()),
		SendID:     in.GetSendId(),
		SendAs:     &sendAs,
		MessageIDs: utils.Map(in.GetMessageIds(), utils.IDsParser),
	}
}

func MapToForwardMessagesResponse(out *dto.ForwardMessagesResponse) *impb.ForwardMessagesResponse {
	if out == nil {
		return nil
	}

	return &impb.ForwardMessagesResponse{
		ThreadId:   out.ThreadID.String(),
		Ids:        uuidsToStrings(out.IDs),
		SkippedIds: uuidsToStrings(out.SkippedIDs),
	}
}

func uuidsToStrings(ids []uuid.UUID) []string {
	return utils.Map(ids, func(id uuid.UUID) string { return id.String() })
}
