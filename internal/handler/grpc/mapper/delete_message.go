package mapper

import (
	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/utils"
)

// ConvertPbDeleteMessagesToDomain maps a DeleteMessagesRequest onto the
// service-layer request.
func ConvertPbDeleteMessagesToDomain(in *impb.DeleteMessagesRequest) *dto.DeleteMessagesRequest {
	if in == nil {
		return nil
	}

	return &dto.DeleteMessagesRequest{
		DeletedBy: MapPeerFromProto(in.GetDeletedBy()),
		IDs:       utils.Map(in.GetIds(), utils.IDsParser),
	}
}

// MapToDeleteMessagesResponse reports which of the requested messages were
// actually deleted and which ones were left untouched, with the reason for
// each.
func MapToDeleteMessagesResponse(out *dto.DeleteMessagesResponse) *impb.DeleteMessagesResponse {
	if out == nil {
		return nil
	}

	skipped := make([]*impb.SkippedMessage, 0, len(out.Skipped))
	for _, skip := range out.Skipped {
		skipped = append(skipped, &impb.SkippedMessage{
			Id:     skip.ID.String(),
			Reason: impb.SkippedMessage_Reason(skip.Reason),
		})
	}

	return &impb.DeleteMessagesResponse{
		DeletedIds: utils.Map(out.DeletedIDs, uuid.UUID.String),
		Skipped:    skipped,
		DeletedAt:  max(out.DeletedAt.UTC().UnixMilli(), 0),
	}
}
