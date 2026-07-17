package dto

import (
	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	SendTextRequest struct {
		From     shared.Peer `json:"from"`
		To       shared.Peer `json:"to"`
		Body     string      `json:"body"`
		DomainID int64       `json:"domain_id"`
		SendID   string      `json:"send_id"`
		SendAs   *uuid.UUID

		ReplyToMessageID  *uuid.UUID `json:"reply_to_message_id,omitempty"`
		ExternalID        string     `json:"external_id,omitempty"`
		ReplyToExternalID string     `json:"reply_to_external_id,omitempty"`
	}

	SendTextResponse struct {
		To shared.Peer `json:"to"`
		ID uuid.UUID   `json:"id"`
	}
)
