package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestSetCRMContact(t *testing.T) {
	t.Parallel()

	operatorContactID := uuid.New()
	clientContactID := uuid.New()
	botContactID := uuid.New()

	members := []*model.ThreadDialogExtended{
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: operatorContactID, ThreadRole: model.RoleMember},
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: clientContactID, ThreadRole: model.RoleOwner},
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: botContactID, ThreadRole: model.RoleMember, IsBot: true},
	}
	participants := []*model.ThreadDialog{{ContactID: operatorContactID}, {ContactID: clientContactID}}

	tests := []struct {
		name      string
		initiator uuid.UUID
		crmID     string
		storeErr  error
		wantCode  codes.Code
		wantSet   []string
	}{
		{name: "operator selects a contact", initiator: operatorContactID, crmID: "42", wantSet: []string{"42"}},
		{name: "operator clears the selection", initiator: operatorContactID, crmID: "", wantSet: []string{""}},
		{name: "client cannot select", initiator: clientContactID, crmID: "42", wantCode: codes.PermissionDenied},
		{name: "bot cannot select", initiator: botContactID, crmID: "42", wantCode: codes.PermissionDenied},
		{name: "non-member cannot select", initiator: uuid.New(), crmID: "42", wantCode: codes.PermissionDenied},
		{
			name: "thread not found", initiator: operatorContactID, crmID: "42",
			storeErr: errors.NotFound("thread not found"), wantCode: codes.NotFound, wantSet: []string{"42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threadID := uuid.New()
			threads := &fakeThreadStore{setCRMContactErr: tt.storeErr}
			outboxStore := &fakeOutboxStore{}

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadStore:       threads,
					threadDialogStore: &fakeThreadDialogStore{fullViewResult: members, quickViewResult: participants},
					outboxStore:       outboxStore,
				},
			}

			err := svc.SetCRMContact(context.Background(), &dto.SetCRMContactRequest{
				ThreadID:           threadID,
				InitiatorContactID: tt.initiator,
				DomainID:           1,
				CRMContactID:       tt.crmID,
			})

			require.Equal(t, tt.wantSet, threads.setCRMContactCalls)

			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, errors.Code(err))
				require.Empty(t, outboxStore.published)

				return
			}

			require.NoError(t, err)
			require.Len(t, outboxStore.published, 1)

			updated, ok := outboxStore.published[0].event.(*event.ThreadUpdated)
			require.True(t, ok)
			require.Equal(t, updated.Topic(), outboxStore.published[0].topic)
			require.Equal(t, threadID, updated.ThreadID)
			require.Equal(t, tt.initiator, updated.UpdatedBy)
			require.Equal(t, tt.crmID, updated.CRMContactID)
			require.Equal(t, int32(1), updated.DomainID)
			require.ElementsMatch(t, []uuid.UUID{operatorContactID, clientContactID}, updated.Participants)
		})
	}
}

func TestSetCRMContact_NilRequest(t *testing.T) {
	t.Parallel()

	err := (&ThreadManagementService{}).SetCRMContact(context.Background(), nil)
	require.Equal(t, codes.InvalidArgument, errors.Code(err))
}
