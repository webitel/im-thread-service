package guards

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestValidateSetReaction(t *testing.T) {
	t.Parallel()

	base := func() *dto.SetReactionRequest {
		return &dto.SetReactionRequest{
			Reactor:   shared.Peer{ID: uuid.New(), Type: shared.PeerContact},
			MessageID: uuid.New(),
			Emoji:     "👍",
		}
	}

	t.Run("valid single emoji passes", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ValidateSetReaction(base()))
	})

	t.Run("emoji with modifiers and ZWJ is one grapheme", func(t *testing.T) {
		t.Parallel()

		for _, e := range []string{"👍🏽", "❤️", "👨‍👩‍👧‍👦", "🇺🇦"} {
			in := base()
			in.Emoji = e
			require.NoError(t, ValidateSetReaction(in), "emoji %q should pass", e)
		}
	})

	t.Run("empty emoji is a clear and passes", func(t *testing.T) {
		t.Parallel()

		in := base()
		in.Emoji = ""
		require.NoError(t, ValidateSetReaction(in))
	})

	t.Run("more than one grapheme is rejected", func(t *testing.T) {
		t.Parallel()

		in := base()
		in.Emoji = "👍👍"
		err := ValidateSetReaction(in)
		require.Error(t, err)
		require.Equal(t, "service.message.set_reaction.not_single", errors.ID(err))
	})

	t.Run("a non-emoji grapheme is rejected", func(t *testing.T) {
		t.Parallel()

		for _, s := range []string{"a", "1", "字"} {
			in := base()
			in.Emoji = s
			err := ValidateSetReaction(in)
			require.Error(t, err, "value %q should be rejected", s)
			require.Equal(t, "service.message.set_reaction.not_emoji", errors.ID(err))
		}
	})

	t.Run("custom emoji is rejected as unsupported", func(t *testing.T) {
		t.Parallel()

		in := base()
		in.Emoji = ""
		in.CustomEmojiID = "doc-1"
		err := ValidateSetReaction(in)
		require.Error(t, err)
		require.Equal(t, "service.message.set_reaction.custom_unsupported", errors.ID(err))
	})

	t.Run("missing message id is rejected", func(t *testing.T) {
		t.Parallel()

		in := base()
		in.MessageID = uuid.Nil
		require.Error(t, ValidateSetReaction(in))
	})

	t.Run("missing reactor is rejected", func(t *testing.T) {
		t.Parallel()

		in := base()
		in.Reactor = shared.Peer{}
		require.Error(t, ValidateSetReaction(in))
	})
}
