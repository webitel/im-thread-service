package dto

import (
	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	// SendTypingRequest is an ephemeral typing indicator for a thread.
	SendTypingRequest struct {
		// From is the participant who is typing.
		From shared.Peer `json:"from"`
		// ThreadID is the target thread.
		ThreadID uuid.UUID `json:"thread_id"`
		// DomainID is the tenant.
		DomainID int64 `json:"domain_id"`
		// TimeoutMS is the requested indicator lifetime; 0 means "use default".
		// The service clamps it to the configured maximum.
		TimeoutMS int32 `json:"timeout_ms"`
		// PreviewText is the optional live draft (Live Typing Preview). An empty
		// value clears any previously shown preview.
		PreviewText string `json:"preview_text,omitempty"`
	}

	// SendTypingResponse is empty: typing is fire-and-forget.
	SendTypingResponse struct{}
)
