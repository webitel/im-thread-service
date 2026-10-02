package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestMessageHistoryService_RejectsAroundCursor(t *testing.T) {
	t.Parallel()

	cursor := &dto.HistoryMessageCursor{ID: uuid.New(), Around: true}

	tests := []struct {
		name   string
		search func(*MessageHistoryService) error
	}{
		{
			name: "search messages",
			search: func(s *MessageHistoryService) error {
				_, _, err := s.SearchMessages(context.Background(), &dto.SearchMessagesInputDTO{
					Term:     "hello",
					CallerID: uuid.New(),
					Cursor:   cursor,
				})

				return err
			},
		},
		{
			name: "left threads",
			search: func(s *MessageHistoryService) error {
				_, _, err := s.SearchLeftThreads(context.Background(), &dto.LeftThreadsMessageHistoryInputDTO{
					ThreadID: uuid.New(),
					Cursor:   cursor,
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.search(&MessageHistoryService{})

			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, errors.Code(err))
		})
	}
}
