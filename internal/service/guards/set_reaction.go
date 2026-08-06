package guards

import (
	"github.com/forPelevin/gomoji"
	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"golang.org/x/text/unicode/norm"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

const setReactionGuardID = "service.message.set_reaction"

// maxReactionBytes bounds a single emoji: a ZWJ sequence with skin-tone
// modifiers and variation selectors (e.g. a family emoji) stays well under this.
const maxReactionBytes = 64

// ValidateSetReaction enforces the hybrid input policy for a reaction write and
// normalizes the emoji in place (NFC). Semantics:
//   - a custom (sticker-backed) emoji is rejected — not yet supported;
//   - an empty emoji is a clear and always allowed;
//   - otherwise the value must be exactly one grapheme cluster that is a valid
//     emoji. Per-messenger allow-lists are applied later, at forward time.
func ValidateSetReaction(in *dto.SetReactionRequest) error {
	if in == nil || in.MessageID == uuid.Nil {
		return errors.InvalidArgument("message id is required", errors.WithID(setReactionGuardID))
	}

	if in.Reactor.ID == uuid.Nil {
		return errors.InvalidArgument("reactor identity is required", errors.WithID(setReactionGuardID))
	}

	if in.CustomEmojiID != "" {
		return errors.InvalidArgument(
			"custom emoji reactions are not supported yet",
			errors.WithID(setReactionGuardID+".custom_unsupported"),
		)
	}

	// An empty emoji clears the reaction — nothing to validate.
	if in.Emoji == "" {
		return nil
	}

	normalized := norm.NFC.String(in.Emoji)

	if len(normalized) > maxReactionBytes {
		return errors.InvalidArgument("reaction emoji is too long", errors.WithID(setReactionGuardID+".too_long"))
	}

	if uniseg.GraphemeClusterCount(normalized) != 1 {
		return errors.InvalidArgument("reaction must be exactly one emoji", errors.WithID(setReactionGuardID+".not_single"))
	}

	if !gomoji.ContainsEmoji(normalized) {
		return errors.InvalidArgument("reaction is not a valid emoji", errors.WithID(setReactionGuardID+".not_emoji"))
	}

	// Persist the canonical (NFC) form so equal emoji dedupe consistently.
	in.Emoji = normalized

	return nil
}
