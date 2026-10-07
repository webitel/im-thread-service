package mapper

import (
	"time"

	"github.com/google/uuid"
	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func ConvertGivePreviewRequest(in *impb.GivePreviewRequest) (*dto.GivePreviewRequest, error) {
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id format", errors.WithCause(err))
	}

	contactID, err := uuid.Parse(in.GetContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid contact_id format", errors.WithCause(err))
	}

	initiatorID, err := uuid.Parse(in.GetInitiatorContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid initiator_contact_id format", errors.WithCause(err))
	}

	return &dto.GivePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           int(in.GetDomainId()),
		Duration:           time.Duration(in.GetDurationMs()) * time.Millisecond,
	}, nil
}

func ConvertRemoveFromPreviewRequest(in *impb.RemoveFromPreviewRequest) (*dto.RemoveFromPreviewRequest, error) {
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id format", errors.WithCause(err))
	}

	contactID, err := uuid.Parse(in.GetContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid contact_id format", errors.WithCause(err))
	}

	req := &dto.RemoveFromPreviewRequest{
		ThreadID:  threadID,
		ContactID: contactID,
		DomainID:  int(in.GetDomainId()),
	}

	if in.InitiatorContactId != nil {
		initiatorID, err := uuid.Parse(in.GetInitiatorContactId())
		if err != nil {
			return nil, errors.InvalidArgument("invalid initiator_contact_id format", errors.WithCause(err))
		}
		req.InitiatorContactID = initiatorID
	}

	return req, nil
}

func ConvertUpgradePreviewRequest(in *impb.UpgradePreviewToFullMemberRequest) (*dto.UpgradePreviewRequest, error) {
	threadID, err := uuid.Parse(in.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id format", errors.WithCause(err))
	}

	contactID, err := uuid.Parse(in.GetContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid contact_id format", errors.WithCause(err))
	}

	initiatorID, err := uuid.Parse(in.GetInitiatorContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid initiator_contact_id format", errors.WithCause(err))
	}

	converter := &ThreadInConverter{}

	return &dto.UpgradePreviewRequest{
		ThreadID:           threadID,
		ContactID:          contactID,
		InitiatorContactID: initiatorID,
		DomainID:           int(in.GetDomainId()),
		Role:               converter.convertMemberRole(in.GetRole()),
	}, nil
}

func ConvertToThreadPreview(p *model.ThreadPreview) *impb.ThreadPreview {
	if p == nil {
		return nil
	}

	return &impb.ThreadPreview{
		Id:                  p.ID.String(),
		ThreadId:            p.ThreadID.String(),
		ContactId:           p.ContactID.String(),
		InitiatorContactId:  p.InitiatorID.String(),
		DomainId:            int32(p.DomainID),
		CreatedAt:           p.CreatedAt.UnixMilli(),
		ExpiresAt:           p.ExpiresAt.UnixMilli(),
	}
}
