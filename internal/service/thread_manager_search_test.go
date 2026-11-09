package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestSearch_SystemCall_Scope(t *testing.T) {
	t.Parallel()

	selfID := uuid.New()
	peerID := uuid.New()

	tests := []struct {
		name          string
		in            *dto.ThreadSearchRequest
		wantErr       bool
		wantSelfScope bool
	}{
		{
			name:          "member search is limited to the caller",
			in:            &dto.ThreadSearchRequest{DomainIDs: []int{1}, SelfID: selfID},
			wantSelfScope: true,
		},
		{
			name:          "system search is not limited to the caller",
			in:            &dto.ThreadSearchRequest{DomainIDs: []int{1}, SelfID: selfID, SystemCall: true},
			wantSelfScope: false,
		},
		{
			name:          "system search by contact is not limited to the caller",
			in:            &dto.ThreadSearchRequest{DomainIDs: []int{1}, SelfID: selfID, ContactIDs: uuid.UUIDs{peerID}, SystemCall: true},
			wantSelfScope: false,
		},
		{
			name:          "system search by participant is not limited to the caller",
			in:            &dto.ThreadSearchRequest{DomainIDs: []int{1}, SelfID: selfID, Participants: []dto.ContactIdentity{{Iss: "portal", Sub: "sub-1"}}, SystemCall: true},
			wantSelfScope: false,
		},
		{
			name:    "system search without domain is rejected",
			in:      &dto.ThreadSearchRequest{SelfID: selfID, SystemCall: true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threads := &fakeThreadStore{}
			svc := &ThreadManagementService{
				uow:      fakeUnitOfWork{threadStore: threads},
				tagHooks: &fakeTagHooks{},
				logger:   slog.Default(),
			}

			_, err := svc.Search(context.Background(), tt.in)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, threads.lastQueryObject)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, threads.lastQueryObject)

			sql, args, err := threads.lastQueryObject.ToSQL()
			require.NoError(t, err)

			if tt.wantSelfScope {
				require.Contains(t, args, selfID)
				require.NotContains(t, sql, "ORDER BY d.is_bot DESC")
			} else {
				require.NotContains(t, args, selfID)
				require.Contains(t, sql, "ORDER BY d.is_bot DESC")
			}
		})
	}
}
