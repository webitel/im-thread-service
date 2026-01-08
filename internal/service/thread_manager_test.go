package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	mock_store "github.com/webitel/im-thread-service/gen/mocks/_internal/store"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	"go.uber.org/mock/gomock"
)

func Test_thread_EnsureDirectThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// TEST SETUP
	var (
		mockUOW         = mock_store.NewMockUnitOfWork(ctrl)
		mockThreadStore = mock_store.NewMockThreadStore(ctrl)
		mockDialogStore = mock_store.NewMockThreadDialogStore(ctrl)
		svc             = NewThreadService(mockUOW)
		ctx             = context.Background()
	)

	t.Run("Success_FoundExisting", func(t *testing.T) {
		req := &dto.EnsureDirectThreadRequest{
			DomainID: 1,
			MemberID: uuid.New(),
			PeerFrom: &model.Peer{ID: uuid.New()},
			PeerTo:   &model.Peer{ID: uuid.New()},
		}
		expectedId := uuid.New()

		mockUOW.EXPECT().ThreadDialogStore().Return(mockDialogStore)
		mockDialogStore.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(expectedId, nil)

		resp, err := svc.EnsureDirectThread(ctx, req)

		assert.NoError(t, err)
		assert.Equal(t, expectedId, resp.ID)
	})

	t.Run("Success_CreateNew", func(t *testing.T) {
		req := &dto.EnsureDirectThreadRequest{
			DomainID: 1,
			MemberID: uuid.New(),
			PeerFrom: &model.Peer{ID: uuid.New()},
			PeerTo:   &model.Peer{ID: uuid.New()},
		}
		newThreadID := uuid.New()

		mockUOW.EXPECT().ThreadDialogStore().Return(mockDialogStore)
		mockDialogStore.EXPECT().Resolve(gomock.Any(), gomock.Any()).Return(uuid.Nil, nil)

		mockUOW.EXPECT().WithinTransaction(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, fn func(ctx context.Context, uow store.UnitOfWork) error) error {
				return fn(ctx, mockUOW)
			},
		)

		mockUOW.EXPECT().ThreadStore().Return(mockThreadStore)
		mockThreadStore.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, m *model.Thread) (*model.Thread, error) {
				m.ID = newThreadID
				return m, nil
			},
		)

		mockUOW.EXPECT().ThreadDialogStore().Return(mockDialogStore)
		mockDialogStore.EXPECT().CreateDirectPair(gomock.Any(), gomock.Any()).Return(nil, nil)

		resp, err := svc.EnsureDirectThread(ctx, req)

		assert.NoError(t, err)
		assert.Equal(t, newThreadID, resp.ID)
	})
}
