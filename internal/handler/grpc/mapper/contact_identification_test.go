package mapper

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestMapIdentifyContactRequestToCommand(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()

	cmd, err := MapIdentifyContactRequestToCommand(&impb.IdentifyContactRequest{
		ThreadId:        threadID.String(),
		DomainId:        7,
		PhoneLastDigits: 9,
		PhoneScope:      impb.PhoneScope_PHONE_SCOPE_PRIMARY_ONLY,
		Criteria: []*impb.IdentificationCriterion{
			{Type: impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_PHONE, Value: "+380000000001"},
			{Type: impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_EMAIL, Value: "qa@example.com"},
			{Type: impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_ATTRIBUTE, Key: "contract", Value: "A-1"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, &model.IdentifyContactCommand{
		ThreadID:        threadID,
		DomainID:        7,
		PhoneLastDigits: 9,
		PhoneScope:      model.PhoneScopePrimaryOnly,
		Criteria: []model.IdentificationCriterion{
			{Type: model.CriterionPhone, Value: "+380000000001"},
			{Type: model.CriterionEmail, Value: "qa@example.com"},
			{Type: model.CriterionAttribute, Key: "contract", Value: "A-1"},
		},
	}, cmd)

	_, err = MapIdentifyContactRequestToCommand(&impb.IdentifyContactRequest{ThreadId: "not-a-uuid", DomainId: 7})
	assert.Equal(t, codes.InvalidArgument, errors.Code(err))
}

func TestMapIdentifyContactResultToProto(t *testing.T) {
	t.Parallel()

	got := MapIdentifyContactResultToProto(&model.IdentifyContactResult{FoundCount: 2, CRMContactID: "10", Trust: model.TrustVerified})
	assert.Equal(t, int32(2), got.GetFoundCount())
	assert.Equal(t, "10", got.GetCrmContactId())
	assert.Equal(t, impb.IdentificationTrust_IDENTIFICATION_TRUST_VERIFIED, got.GetTrust())
}

func TestMapContactCandidatesToProto(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	got := MapContactCandidatesToProto(model.ContactCandidates{{
		CRMContactID: "10",
		Trust:        model.TrustVerified,
		CreatedAt:    at,
		UpdatedAt:    at.Add(time.Second),
		Reasons: []model.MatchReason{
			{Type: model.CriterionMessengerID, Trust: model.TrustVerified, Origin: model.OriginChannel},
			{Type: model.CriterionAttribute, Trust: model.TrustUnverified, Origin: model.OriginFlow, Key: "contract"},
		},
	}}, model.CRMContactBinding{CRMContactID: "10", Source: model.CRMContactSourceSearched})

	assert.Equal(t, "10", got.GetCrmContactId())
	assert.Equal(t, impb.CrmContactSource_CRM_CONTACT_SOURCE_SEARCHED, got.GetCrmContactSource())
	require.Len(t, got.GetCandidates(), 1)

	c := got.GetCandidates()[0]
	assert.Equal(t, "10", c.GetCrmContactId())
	assert.Equal(t, impb.IdentificationTrust_IDENTIFICATION_TRUST_VERIFIED, c.GetTrust())
	assert.Equal(t, at.UnixMilli(), c.GetCreatedAt())
	assert.Equal(t, at.Add(time.Second).UnixMilli(), c.GetUpdatedAt())
	require.Len(t, c.GetReasons(), 2)
	assert.Equal(t, impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_MESSENGER_ID, c.GetReasons()[0].GetType())
	assert.Equal(t, impb.IdentificationOrigin_IDENTIFICATION_ORIGIN_CHANNEL, c.GetReasons()[0].GetOrigin())
	assert.Equal(t, impb.IdentificationTrust_IDENTIFICATION_TRUST_UNVERIFIED, c.GetReasons()[1].GetTrust())
	assert.Equal(t, "contract", c.GetReasons()[1].GetKey())
}

// The model enums are converted by number: they must keep the proto numbers.
func TestContactIdentificationEnumsMatchTheProto(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int32(impb.IdentificationTrust_IDENTIFICATION_TRUST_UNVERIFIED), int32(model.TrustUnverified))
	assert.Equal(t, int32(impb.IdentificationTrust_IDENTIFICATION_TRUST_VERIFIED), int32(model.TrustVerified))
	assert.Equal(t, int32(impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_PHONE), int32(model.CriterionPhone))
	assert.Equal(t, int32(impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_EMAIL), int32(model.CriterionEmail))
	assert.Equal(t, int32(impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_ATTRIBUTE), int32(model.CriterionAttribute))
	assert.Equal(t, int32(impb.IdentificationCriterionType_IDENTIFICATION_CRITERION_TYPE_MESSENGER_ID), int32(model.CriterionMessengerID))
	assert.Equal(t, int32(impb.IdentificationOrigin_IDENTIFICATION_ORIGIN_CHANNEL), int32(model.OriginChannel))
	assert.Equal(t, int32(impb.IdentificationOrigin_IDENTIFICATION_ORIGIN_FLOW), int32(model.OriginFlow))
	assert.Equal(t, int32(impb.PhoneScope_PHONE_SCOPE_PRIMARY_ONLY), int32(model.PhoneScopePrimaryOnly))
	assert.Equal(t, int32(impb.CrmContactSource_CRM_CONTACT_SOURCE_AUTO), int32(model.CRMContactSourceAuto))
	assert.Equal(t, int32(impb.CrmContactSource_CRM_CONTACT_SOURCE_SELECTED), int32(model.CRMContactSourceSelected))
	assert.Equal(t, int32(impb.CrmContactSource_CRM_CONTACT_SOURCE_CREATED), int32(model.CRMContactSourceCreated))
	assert.Equal(t, int32(impb.CrmContactSource_CRM_CONTACT_SOURCE_SEARCHED), int32(model.CRMContactSourceSearched))
}
