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
		From     model.Peer
		To       model.Peer
		DomainID int32
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

func NewCanSendRequestDtoFromPeers(from, to model.Peer, domainID int32) *CanSendRequest {
	return &CanSendRequest{
		From:     from,
		To:       to,
		DomainID: domainID,
	}
}

func NewCanSendResponse(canSend bool) *CanSendResponse {
	return &CanSendResponse{
		CanSend: canSend,
	}
}
