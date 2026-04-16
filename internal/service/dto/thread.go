package dto

import (
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type (
	EnsureDirectThreadRequest struct {
		DomainID int
		From     *shared.Peer
		To       *shared.Peer
	}

	EnsureDirectThreadResponse struct {
		ID       uuid.UUID
		DomainID int32
		Members  uuid.UUIDs
	}

	SearchThreadDialogRequest struct {
		DomainID int
		Kind     model.ThreadKind
		From     *shared.Peer
		To       *shared.Peer
	}

	CreateGroupRequest struct {
		Subject     string
		Description string
		MemberIDs   []string
	}

	ThreadSearchRequest struct {
		Fields     []string
		Ids        uuid.UUIDs
		DomainIDs  []int
		Kinds      []model.ThreadKind
		Owners     uuid.UUIDs
		Q          string
		ContactIDs uuid.UUIDs
		Size       int
		Sort       string
		Page       int
	}

	ThreadMembersResponse struct {
		Members uuid.UUIDs `db:"members"`
	}
)

type AddMemberRequest struct {
	ThreadID           uuid.UUID
	NewMemberContactID uuid.UUID
	InitiatorContactID uuid.UUID
	NewMemberRole      model.ThreadRole
}
type RemoveMemberRequest struct {
	TargetMemberID     uuid.UUID
	InitiatorContactID uuid.UUID
}

func NewEnsureDirectThreadResponse(id uuid.UUID, domainID int32, members uuid.UUIDs) *EnsureDirectThreadResponse {
	return &EnsureDirectThreadResponse{
		ID:       id,
		DomainID: domainID,
		Members:  members,
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
