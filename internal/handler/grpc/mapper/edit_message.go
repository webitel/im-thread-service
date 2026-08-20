package mapper

import (
	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

// ConvertPbEditMessageToDomain maps an EditMessageRequest onto the domain
// message that should be edited.
func ConvertPbEditMessageToDomain(in *impb.EditMessageRequest) *model.Message {
	if in == nil {
		return nil
	}

	id, _ := uuid.Parse(in.GetId())

	return &model.Message{
		ID:   id,
		From: MapPeerFromProto(in.GetEditedBy()),
		Body: in.GetBody(),
	}
}

// MapToEditMessageResponse maps an edited domain message back to the
// EditMessageResponse: the message id, the server-authoritative edit time and
// the history position the new body took.
func MapToEditMessageResponse(msg *model.Message) *impb.EditMessageResponse {
	if msg == nil {
		return nil
	}

	return &impb.EditMessageResponse{
		Id:       msg.ID.String(),
		EditedAt: msg.UpdatedAtUnixMillis(),
		Version:  msg.Version,
	}
}
