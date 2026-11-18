package contacts

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	crmpb "github.com/webitel/im-thread-service/gen/go/webitel/contacts"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestSearchRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		field      model.CRMSearchField
		wantQin    string
		wantFields []string
	}{
		{model.CRMSearchIMContact, "imclients{im_contact_id}", []string{"id"}},
		{model.CRMSearchPhones, "phones", []string{"id", "phones.size(64){number,primary}"}},
		{model.CRMSearchEmails, "emails", []string{"id", "emails.size(64){email}"}},
		{model.CRMSearchVariables, "variables", []string{"id", "variables.size(64){key,value}"}},
	}

	for _, tt := range tests {
		req, err := searchRequest(model.CRMContactQuery{DomainID: 7, Field: tt.field, Term: "term", Size: 100, Page: 2})
		require.NoError(t, err)
		assert.Equal(t, int64(7), req.GetDomainId())
		assert.Equal(t, "term", req.GetQ())
		assert.Equal(t, []string{tt.wantQin}, req.GetQin())
		assert.Equal(t, tt.wantFields, req.GetFields())
		assert.Equal(t, int32(100), req.GetSize())
		assert.Equal(t, int32(2), req.GetPage())
	}

	req, err := searchRequest(model.CRMContactQuery{DomainID: 7, Field: model.CRMSearchPhones, Term: "1"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), req.GetPage(), "pages start at 1")

	_, err = searchRequest(model.CRMContactQuery{DomainID: 7, Term: "x"})
	require.Error(t, err, "unknown field")

	_, err = searchRequest(model.CRMContactQuery{Field: model.CRMSearchPhones, Term: "x"})
	require.Error(t, err, "no domain")
}

func TestLinkError(t *testing.T) {
	t.Parallel()

	require.NoError(t, linkError("1", nil))

	linked := linkError("1", status.Error(codes.AlreadyExists, "contacts.imclients.im_contact_id.conflict"))
	require.ErrorIs(t, linked, model.ErrIMContactLinked)
	assert.Equal(t, codes.AlreadyExists, status.Code(linked), "the CRM status is kept")

	other := linkError("1", status.Error(codes.Unavailable, "down"))
	require.NotErrorIs(t, other, model.ErrIMContactLinked)
	assert.Equal(t, codes.Unavailable, status.Code(other))

	require.NotErrorIs(t, linkError("1", errors.New("boom")), model.ErrIMContactLinked)
}

func TestToModel(t *testing.T) {
	t.Parallel()

	got := toModel(&crmpb.Contact{
		Id:     "10",
		Phones: &crmpb.PhoneList{Data: []*crmpb.PhoneNumber{{Number: "+380000000001", Primary: true}}},
		Emails: &crmpb.EmailList{Data: []*crmpb.EmailAddress{{Email: "qa@example.com"}}},
		Variables: &crmpb.VariableList{Data: []*crmpb.Variable{
			{Key: "s", Value: structpb.NewStringValue("A-1")},
			{Key: "n", Value: structpb.NewNumberValue(42)},
			{Key: "b", Value: structpb.NewBoolValue(true)},
			{Key: "l", Value: structpb.NewListValue(&structpb.ListValue{})},
		}},
	})

	assert.Equal(t, &model.CRMContact{
		ID:        "10",
		Phones:    []model.CRMPhone{{Number: "+380000000001", Primary: true}},
		Emails:    []string{"qa@example.com"},
		Variables: map[string]string{"s": "A-1", "n": "42", "b": "true", "l": ""},
	}, got)
}
