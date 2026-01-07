package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	EnsureDirectThreadRequest struct {
		DomainID int
		MemberID uuid.UUID
		PeerFrom *model.Peer
		PeerTo   *model.Peer
	}

	EnsureDirectThreadResponse struct {
		ID uuid.UUID
	}

	SearchThreadDialogRequest struct {
		DomainID int
		Kind     model.ThreadKind
		From     *model.Peer
		To       *model.Peer
	}

	CanSendRequest struct {
		FromID string
		ToID   string
	}

	CanSendResponse struct {
		CanSend bool
	}
)

func NewEnsureDirectThreadResponse(id uuid.UUID) *EnsureDirectThreadResponse {
	return &EnsureDirectThreadResponse{
		ID: id,
	}
}

func NewSearchThreadRequest(domainID int, kind model.ThreadKind, from, to *model.Peer) *SearchThreadDialogRequest {
	return &SearchThreadDialogRequest{
		DomainID: domainID,
		Kind:     kind,
		From:     from,
		To:       to,
	}
}

func NewCanSendRequestDtoFromPeers(from, to model.Peer) *CanSendRequest {
	return &CanSendRequest{
		FromID: from.ID.String(),
		ToID:   to.ID.String(),
	}
}

func NewCanSendResponse(canSend bool) *CanSendResponse {
	return &CanSendResponse{
		CanSend: canSend,
	}
}
