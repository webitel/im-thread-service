package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	// SetReactionRequest sets, replaces or clears the reactor's single emoji
	// reaction on a message. An empty Emoji clears it.
	SetReactionRequest struct {
		Reactor   shared.Peer
		MessageID uuid.UUID
		ThreadID  *uuid.UUID

		// Emoji is the unicode reaction; empty means clear. CustomEmojiID is set
		// only when the caller sent the (not-yet-supported) custom-emoji arm, so
		// the guard can reject it explicitly rather than silently clearing.
		Emoji         string
		CustomEmojiID string

		DomainID       int32
		IdempotencyKey string
		ExternalID     string
	}

	// SetReactionResponse reports what the call settled on relative to the
	// reactor's previous reaction.
	SetReactionResponse struct {
		Action    model.ReactionAction
		Emoji     string
		ReactedAt time.Time
	}
)
