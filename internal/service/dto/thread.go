package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type (
	EnsureDirectThreadRequest struct {
		DomainId int
		MemberId uuid.UUID
		PeerFrom *model.Peer
		PeerTo *model.Peer
	}

	EnsureDirectThreadResponse struct {
		Id uuid.UUID
	}

	SearchThreadDialogRequest struct {
		DomainId int
		Kind model.ThreadKind
		From *model.Peer
		To *model.Peer
	}

	CanSendRequest struct {
		FromId string
		ToId string
	}

	CanSendResponse struct {
		CanSend bool
	}
)

func NewEnsureDirectThreadResponse(id uuid.UUID) *EnsureDirectThreadResponse {
	return &EnsureDirectThreadResponse{
		Id: id,
	}
}

func NewSearchThreadRequest(domainId int, kind model.ThreadKind, from, to *model.Peer) *SearchThreadDialogRequest {
	return &SearchThreadDialogRequest{
		DomainId: domainId,
		Kind: kind,
		From: from,
		To: to,
	}
}

func NewCanSendRequestDtoFromPeers(from, to model.Peer) *CanSendRequest {
	return &CanSendRequest{
		FromId: from.Id.String(),
		ToId: to.Id.String(),
	}
}

func NewCanSendResponse(canSend bool) *CanSendResponse {
	return &CanSendResponse{
		CanSend: canSend,
	}
}