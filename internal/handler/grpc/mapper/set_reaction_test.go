package mapper

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestConvertPbSetReactionToDomain(t *testing.T) {
	t.Parallel()

	reactorID := uuid.New()
	messageID := uuid.New()
	threadID := uuid.New()
	threadStr := threadID.String()

	in := &impb.SetReactionRequest{
		Reactor:    &impb.Peer{Kind: &impb.Peer_ContactId{ContactId: reactorID.String()}},
		MessageId:  messageID.String(),
		ThreadId:   &threadStr,
		Reaction:   &impb.ReactionContent{Kind: &impb.ReactionContent_Emoji{Emoji: "❤️"}},
		DomainId:   7,
		SendId:     "send-42",
		ExternalId: "ext-9",
	}

	out := ConvertPbSetReactionToDomain(in)
	require.NotNil(t, out)
	require.Equal(t, reactorID, out.Reactor.ID)
	require.Equal(t, messageID, out.MessageID)
	require.NotNil(t, out.ThreadID)
	require.Equal(t, threadID, *out.ThreadID)
	require.Equal(t, "❤️", out.Emoji)
	require.Empty(t, out.CustomEmojiID)
	require.Equal(t, int32(7), out.DomainID)
	require.Equal(t, "send-42", out.IdempotencyKey)
	require.Equal(t, "ext-9", out.ExternalID)

	require.Nil(t, ConvertPbSetReactionToDomain(nil))

	// The custom-emoji arm is carried through (the guard rejects it later),
	// rather than collapsing to an empty clear.
	custom := ConvertPbSetReactionToDomain(&impb.SetReactionRequest{
		Reactor:   &impb.Peer{Kind: &impb.Peer_ContactId{ContactId: reactorID.String()}},
		MessageId: messageID.String(),
		Reaction:  &impb.ReactionContent{Kind: &impb.ReactionContent_CustomEmojiId{CustomEmojiId: "doc-1"}},
	})
	require.Empty(t, custom.Emoji)
	require.Equal(t, "doc-1", custom.CustomEmojiID)
}

func TestMapToSetReactionResponse(t *testing.T) {
	t.Parallel()

	out := MapToSetReactionResponse(&dto.SetReactionResponse{
		Action:    model.ReactionActionSet,
		Emoji:     "👍",
		ReactedAt: time.UnixMilli(1234).UTC(),
	})
	require.NotNil(t, out)
	require.Equal(t, impb.ReactionAction_REACTION_ACTION_SET, out.GetAction())
	require.Equal(t, "👍", out.GetReaction().GetEmoji())
	require.Equal(t, int64(1234), out.GetReactedAt())

	// A removed reaction carries no content.
	removed := MapToSetReactionResponse(&dto.SetReactionResponse{Action: model.ReactionActionRemoved})
	require.Equal(t, impb.ReactionAction_REACTION_ACTION_REMOVED, removed.GetAction())
	require.Nil(t, removed.GetReaction())

	require.Nil(t, MapToSetReactionResponse(nil))
}

func TestMapReactionActionToProto(t *testing.T) {
	t.Parallel()

	cases := map[model.ReactionAction]impb.ReactionAction{
		model.ReactionActionSet:         impb.ReactionAction_REACTION_ACTION_SET,
		model.ReactionActionRemoved:     impb.ReactionAction_REACTION_ACTION_REMOVED,
		model.ReactionActionUnchanged:   impb.ReactionAction_REACTION_ACTION_UNCHANGED,
		model.ReactionActionUnspecified: impb.ReactionAction_REACTION_ACTION_UNSPECIFIED,
	}

	for in, want := range cases {
		require.Equal(t, want, mapReactionActionToProto(in))
	}
}

func TestMapReactions(t *testing.T) {
	t.Parallel()

	require.Empty(t, mapReactions(nil, uuid.Nil))

	me := uuid.New()
	other := uuid.New()
	out := mapReactions([]*model.MessageReaction{
		{Emoji: "👍", Count: 2, ReactorIDs: []uuid.UUID{me, other}, LastReactedAt: 5000},
		{Emoji: "❤️", Count: 1, ReactorIDs: []uuid.UUID{other}, LastReactedAt: 6000},
	}, me)

	require.Len(t, out, 2)

	require.Equal(t, "👍", out[0].GetReaction().GetEmoji())
	require.Equal(t, int32(2), out[0].GetCount())
	require.True(t, out[0].GetReactedByMe())
	require.ElementsMatch(t, []string{me.String(), other.String()}, out[0].GetReactorIds())
	require.Equal(t, int64(5000), out[0].GetLastReactedAt())

	// Caller did not react with ❤️.
	require.False(t, out[1].GetReactedByMe())
}
