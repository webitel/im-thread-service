package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	EnsureDirectThreadRequest struct {
		DomainID int
		MemberID uuid.UUID
		PeerFrom *shared.Peer
		PeerTo   *shared.Peer
	}

	EnsureDirectThreadResponse struct {
		ID uuid.UUID
	}

	SearchThreadDialogRequest struct {
		DomainID int
		Kind     model.ThreadKind
		From     *shared.Peer
		To       *shared.Peer
	}

	CanSendRequest struct {
		From     shared.Peer
		To       shared.Peer
		DomainID int32
	}

	CanSendResponse struct {
		CanSend bool
	}

	SearchThreadRequest struct {
		Fields    []string
		Ids       uuid.UUIDs
		DomainIds []int
		Kinds     []model.ThreadKind
		Owners    uuid.UUIDs
		Q         string
		MemberIds uuid.UUIDs
		Limit     int
		Sort      string
		Page      int
	}
)

func NewEnsureDirectThreadResponse(id uuid.UUID) *EnsureDirectThreadResponse {
	return &EnsureDirectThreadResponse{
		ID: id,
	}
}

func NewSearchThreadRequest(domainID int, kind model.ThreadKind, from, to *shared.Peer) *SearchThreadDialogRequest {
	return &SearchThreadDialogRequest{
		DomainID: domainID,
		Kind:     kind,
		From:     from,
		To:       to,
	}
}

func NewCanSendRequestDtoFromPeers(from, to shared.Peer, domainID int32) *CanSendRequest {
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
