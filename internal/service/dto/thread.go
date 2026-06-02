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
		SendAs   *uuid.UUID
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

	ThreadGetRequest struct {
		ID       uuid.UUID
		DomainID int
		Fields   []string
	}

	ThreadSearchRequest struct {
		Fields     []string
		IDs        uuid.UUIDs
		DomainIDs  []int
		Kinds      []model.ThreadKind
		Owners     uuid.UUIDs
		Q          string
		SelfID     uuid.UUID
		ContactIDs uuid.UUIDs
		Size       int
		Sort       string
		Page       int
	}

	SearchLeftRequest struct {
		Fields   []string
		MemberID uuid.UUID
		DomainID int
		Kinds    []model.ThreadKind
		Size     int
		Sort     string
		Page     int
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
	Reason             *string
}
type TransferThreadRequest struct {
	ThreadID           uuid.UUID
	NewMemberContactID uuid.UUID
	InitiatorContactID uuid.UUID
	NewMemberRole      model.ThreadRole
}

func NewSearchThreadRequest(domainID int, kind model.ThreadKind, from, to *shared.Peer) *SearchThreadDialogRequest {
	return &SearchThreadDialogRequest{
		DomainID: domainID,
		Kind:     kind,
		From:     from,
		To:       to,
	}
}
