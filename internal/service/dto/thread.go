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
		// ToIsBot is evaluated lazily — called only when creating a new thread.
		// Pass nil to treat the peer as non-bot.
		ToIsBot func() bool
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
		Fields       []string
		IDs          uuid.UUIDs
		DomainIDs    []int
		Kinds        []model.ThreadKind
		Owners       uuid.UUIDs
		Q            string
		SelfID       uuid.UUID
		ContactIDs   uuid.UUIDs
		Participants []ContactIdentity
		Size         int
		Sort         string
		Page         int
	}

	ContactIdentity struct {
		Sub string
		Iss string
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

	UnreadSummaryRequest struct {
		SelfID   uuid.UUID
		DomainID int32
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
	DomainID           int
	IsBot              bool
	AutoLeave          *bool
}

type RemoveMemberRequest struct {
	TargetMemberID     uuid.UUID
	InitiatorContactID uuid.UUID
	Reason             *string
}

// CompleteBotControlRequest is sent by flow_manager when a bot schema finishes execution.
// It signals that the bot voluntarily releases control (reason: completed).
type CompleteBotControlRequest struct {
	ThreadID uuid.UUID
	MemberID uuid.UUID
	DomainID int
}

// ReleaseBotControlRequest is used when a user stops the active bot from the thread
type ReleaseBotControlRequest struct {
	ThreadID          uuid.UUID
	InitiatorMemberID uuid.UUID
	DomainID          int
}

type TransferThreadRequest struct {
	ThreadID           uuid.UUID
	NewMemberContactID uuid.UUID
	InitiatorContactID uuid.UUID
	NewMemberRole      model.ThreadRole
	TargetIsBot        bool
	AutoLeave          *bool
}

func NewSearchThreadRequest(domainID int, kind model.ThreadKind, from, to *shared.Peer) *SearchThreadDialogRequest {
	return &SearchThreadDialogRequest{
		DomainID: domainID,
		Kind:     kind,
		From:     from,
		To:       to,
	}
}
