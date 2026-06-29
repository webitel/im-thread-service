package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestReleaseBotControl_PopsActiveBotAndPublishesReleased(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()
	initiatorMemberID := uuid.New()

	botControl := &fakeBotControlStore{
		newTopEntry: nil, // stack becomes empty after release
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &botMemberID, Position: 0},
		},
	}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: &fakeThreadDialogStore{},
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
	}

	err := svc.ReleaseBotControl(context.Background(), &dto.ReleaseBotControlRequest{
		ThreadID:          threadID,
		InitiatorMemberID: initiatorMemberID,
		DomainID:          1,
	})
	require.NoError(t, err)

	require.Equal(t, botMemberID, botControl.lastPopMemberID, "the active bot must be popped")
	require.Equal(t, model.BotControlReasonClientLeave, botControl.lastPopReason)

	released := findReleasedEvent(outboxStore)
	require.NotNil(t, released, "BotControlReleased must be published")
	require.Equal(t, botMemberID, released.MemberID)
	require.Equal(t, string(model.BotControlReasonClientLeave), released.Reason)

	require.Nil(t, findGrantedEvent(outboxStore), "no grant when the stack becomes empty")
}

func TestReleaseBotControl_ReturnsControlToPreviousBot(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()
	newTopMemberID := uuid.New()
	newTopContactID := uuid.New()

	newTop := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &newTopMemberID, ContactID: newTopContactID, Position: 0,
	}
	botControl := &fakeBotControlStore{
		newTopEntry: newTop,
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &botMemberID, Position: 1},
		},
	}
	outboxStore := &fakeOutboxStore{}

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel: shared.BaseModel{ID: newTopMemberID, DomainID: 1},
				ContactID: newTopContactID,
				ThreadID:  threadID,
			},
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
	}

	err := svc.ReleaseBotControl(context.Background(), &dto.ReleaseBotControlRequest{
		ThreadID: threadID,
		DomainID: 1,
	})
	require.NoError(t, err)

	require.Equal(t, model.BotControlReasonClientLeave, botControl.lastPopReason)

	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted, "control must be returned to the previous controller")
	require.Equal(t, newTopMemberID, granted.MemberID)
	require.True(t, granted.IsResume, "returning control to a previous bot is a resume")
	require.Equal(t, string(model.BotControlReasonClientLeave), granted.Reason)
}

func TestReleaseBotControl_EmptyStack_NoOp(t *testing.T) {
	botControl := &fakeBotControlStore{stackResult: nil}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: &fakeThreadDialogStore{},
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
	}

	err := svc.ReleaseBotControl(context.Background(), &dto.ReleaseBotControlRequest{
		ThreadID: uuid.New(),
		DomainID: 1,
	})
	require.NoError(t, err, "/close on a thread with no active bot is a no-op")

	require.Nil(t, findReleasedEvent(outboxStore))
	require.Equal(t, uuid.Nil, botControl.lastPopMemberID, "Pop must not be called on an empty stack")
}

func TestReleaseBotControl_NilRequest(t *testing.T) {
	svc := &ThreadManagementService{}
	require.Error(t, svc.ReleaseBotControl(context.Background(), nil))
}
