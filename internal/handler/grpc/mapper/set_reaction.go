package mapper

import (
	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// ConvertPbSetReactionToDomain maps a SetReactionRequest onto the service-layer
// request. The message id is proto-validated as a uuid, so a parse error yields
// the nil uuid the service then rejects.
func ConvertPbSetReactionToDomain(in *impb.SetReactionRequest) *dto.SetReactionRequest {
	if in == nil {
		return nil
	}

	messageID, _ := uuid.Parse(in.GetMessageId())

	out := &dto.SetReactionRequest{
		Reactor:        MapPeerFromProto(in.GetReactor()),
		MessageID:      messageID,
		ThreadID:       ParseOptionalUUID(in.GetThreadId()),
		DomainID:       int32(in.GetDomainId()),
		IdempotencyKey: in.GetSendId(),
		ExternalID:     in.GetExternalId(),
	}

	// Distinguish the union arms: a custom-emoji reaction is carried through so
	// the guard can reject it, rather than collapsing to an empty clear.
	switch kind := in.GetReaction().GetKind().(type) {
	case *impb.ReactionContent_Emoji:
		out.Emoji = kind.Emoji
	case *impb.ReactionContent_CustomEmojiId:
		out.CustomEmojiID = kind.CustomEmojiId
	}

	return out
}

// MapToSetReactionResponse reports what the reaction call settled on.
func MapToSetReactionResponse(out *dto.SetReactionResponse) *impb.SetReactionResponse {
	if out == nil {
		return nil
	}

	resp := &impb.SetReactionResponse{
		Action:    mapReactionActionToProto(out.Action),
		ReactedAt: max(out.ReactedAt.UTC().UnixMilli(), 0),
	}

	// A removed reaction carries no content; a set/unchanged one echoes the emoji.
	if out.Emoji != "" {
		resp.Reaction = &impb.ReactionContent{
			Kind: &impb.ReactionContent_Emoji{Emoji: out.Emoji},
		}
	}

	return resp
}

func mapReactionActionToProto(action model.ReactionAction) impb.ReactionAction {
	switch action {
	case model.ReactionActionSet:
		return impb.ReactionAction_REACTION_ACTION_SET
	case model.ReactionActionRemoved:
		return impb.ReactionAction_REACTION_ACTION_REMOVED
	case model.ReactionActionUnchanged:
		return impb.ReactionAction_REACTION_ACTION_UNCHANGED
	case model.ReactionActionUnspecified:
		return impb.ReactionAction_REACTION_ACTION_UNSPECIFIED
	default:
		return impb.ReactionAction_REACTION_ACTION_UNSPECIFIED
	}
}
