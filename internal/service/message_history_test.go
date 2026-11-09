package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type fakeMessageHistoryStore struct {
	sql string
}

func (f *fakeMessageHistoryStore) Search(_ context.Context, query queryobject.QueryObject) ([]*model.Message, error) {
	sql, _, err := query.ToSQL()
	f.sql = sql

	return nil, err
}

func TestMessageHistoryService_Search_Access(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	callerID := uuid.New()

	tests := []struct {
		name          string
		in            *dto.HistoryMessageInputDTO
		wantErr       bool
		wantCallerACL bool
	}{
		{
			name:          "member read applies caller limitation",
			in:            &dto.HistoryMessageInputDTO{DomainID: 1, ThreadIDs: uuid.UUIDs{threadID}, CallerID: callerID},
			wantCallerACL: true,
		},
		{
			name:    "member read without caller is rejected",
			in:      &dto.HistoryMessageInputDTO{DomainID: 1, ThreadIDs: uuid.UUIDs{threadID}},
			wantErr: true,
		},
		{
			name:          "system read skips caller limitation",
			in:            &dto.HistoryMessageInputDTO{DomainID: 1, ThreadIDs: uuid.UUIDs{threadID}, CallerID: callerID, SystemCall: true},
			wantCallerACL: false,
		},
		{
			name:          "system read without caller is allowed",
			in:            &dto.HistoryMessageInputDTO{DomainID: 1, ThreadIDs: uuid.UUIDs{threadID}, SystemCall: true},
			wantCallerACL: false,
		},
		{
			name:    "system read without domain is rejected",
			in:      &dto.HistoryMessageInputDTO{ThreadIDs: uuid.UUIDs{threadID}, CallerID: callerID, SystemCall: true},
			wantErr: true,
		},
		{
			name:    "nil request is rejected",
			in:      nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeMessageHistoryStore{}
			svc := &MessageHistoryService{messageHistoryStore: store}

			_, _, err := svc.Search(context.Background(), tt.in)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, store.sql)

				return
			}

			require.NoError(t, err)
			require.Contains(t, store.sql, "domain_id")

			if tt.wantCallerACL {
				require.Contains(t, store.sql, "from im_thread.thread_dialog acl")
			} else {
				require.NotContains(t, store.sql, "from im_thread.thread_dialog acl")
			}
		})
	}
}
