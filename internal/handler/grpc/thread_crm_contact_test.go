package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type recordingThreadManager struct {
	ThreadManagementService

	got *dto.SetCRMContactRequest
}

func (r *recordingThreadManager) SetCRMContact(_ context.Context, req *dto.SetCRMContactRequest) error {
	r.got = req

	return nil
}

func TestSetCrmContact_MapsTheRequest(t *testing.T) {
	t.Parallel()

	threadID, operator := uuid.New(), uuid.New()
	manager := &recordingThreadManager{}
	srv := &ThreadManagementServer{threadManager: manager}

	resp, err := srv.SetCrmContact(context.Background(), &impb.SetCrmContactRequest{
		ThreadId:           threadID.String(),
		InitiatorContactId: operator.String(),
		DomainId:           7,
		CrmContactId:       "42",
		Source:             impb.CrmContactSource_CRM_CONTACT_SOURCE_CREATED,
		LinkImContact:      true,
	})
	require.NoError(t, err)
	require.Equal(t, "42", resp.GetCrmContactId())
	require.Equal(t, &dto.SetCRMContactRequest{
		ThreadID:           threadID,
		InitiatorContactID: operator,
		DomainID:           7,
		CRMContactID:       "42",
		Source:             model.CRMContactSourceCreated,
		LinkIMContact:      true,
	}, manager.got)
}
