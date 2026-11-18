package mapper

import (
	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func MapIdentifyContactRequestToCommand(req *impb.IdentifyContactRequest) (*model.IdentifyContactCommand, error) {
	threadID, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id", errors.WithCause(err))
	}

	cmd := &model.IdentifyContactCommand{
		ThreadID:        threadID,
		DomainID:        int(req.GetDomainId()),
		PhoneLastDigits: int(req.GetPhoneLastDigits()),
		PhoneScope:      model.PhoneScope(req.GetPhoneScope()),
		Criteria:        make([]model.IdentificationCriterion, 0, len(req.GetCriteria())),
	}

	for _, c := range req.GetCriteria() {
		cmd.Criteria = append(cmd.Criteria, model.IdentificationCriterion{
			Type:  model.CriterionType(c.GetType()),
			Value: c.GetValue(),
			Key:   c.GetKey(),
		})
	}

	return cmd, nil
}

func MapIdentifyContactResultToProto(result *model.IdentifyContactResult) *impb.IdentifyContactResponse {
	return &impb.IdentifyContactResponse{
		FoundCount:   int32(min(result.FoundCount, 2*model.MaxCandidatesPerThread)), //nolint:gosec // G115: bounded by the candidate limit
		CrmContactId: result.CRMContactID,
		Trust:        impb.IdentificationTrust(result.Trust),
	}
}

func MapContactCandidatesToProto(candidates model.ContactCandidates, binding model.CRMContactBinding) *impb.ListContactCandidatesResponse {
	resp := &impb.ListContactCandidatesResponse{
		CrmContactId:     binding.CRMContactID,
		CrmContactSource: impb.CrmContactSource(binding.Source),
		Candidates:       make([]*impb.ContactCandidate, 0, len(candidates)),
	}

	for _, c := range candidates {
		out := &impb.ContactCandidate{
			CrmContactId: c.CRMContactID,
			Trust:        impb.IdentificationTrust(c.Trust),
			CreatedAt:    c.CreatedAt.UnixMilli(),
			UpdatedAt:    c.UpdatedAt.UnixMilli(),
			Reasons:      make([]*impb.ContactMatchReason, 0, len(c.Reasons)),
		}

		for _, r := range c.Reasons {
			out.Reasons = append(out.Reasons, &impb.ContactMatchReason{
				Type:   impb.IdentificationCriterionType(r.Type),
				Trust:  impb.IdentificationTrust(r.Trust),
				Origin: impb.IdentificationOrigin(r.Origin),
				Key:    r.Key,
			})
		}

		resp.Candidates = append(resp.Candidates, out)
	}

	return resp
}
