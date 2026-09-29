package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func TestAddMember_Operator(t *testing.T) {
	t.Parallel()

	ownerBotMemberID := uuid.New()
	ownerBotContactID := uuid.New()
	ownerBotSub := int64(77)

	tests := []struct {
		name         string
		role         model.ThreadRole
		controller   *uuid.UUID
		wantClears   int
		wantReleased bool
	}{
		{name: "operator takes control from the bot", role: model.RoleMember, controller: &ownerBotMemberID, wantClears: 1, wantReleased: true},
		{name: "operator joins a thread no bot controls", role: model.RoleMember, wantClears: 1},
		{name: "owner joining leaves bot control alone", role: model.RoleOwner},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threadID := uuid.New()
			botControl := &fakeBotControlStore{clearedMemberID: tt.controller}
			outboxStore := &fakeOutboxStore{}

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadDialogStore: &fakeThreadDialogStore{
						quickViewResult: []*model.ThreadDialog{
							{BaseModel: shared.BaseModel{ID: ownerBotMemberID}, ContactID: ownerBotContactID, ThreadID: threadID, ThreadRole: model.RoleOwner, IsBot: true},
						},
					},
					messageStore:    &fakeMessageStore{},
					outboxStore:     outboxStore,
					botControlStore: botControl,
				},
				contactInfo: &fakeContactInfo{subs: map[uuid.UUID]int64{ownerBotContactID: ownerBotSub}},
			}

			_, err := svc.AddMember(context.Background(), &dto.AddMemberRequest{
				ThreadID:           threadID,
				NewMemberContactID: uuid.New(),
				NewMemberRole:      tt.role,
				DomainID:           1,
				SystemCall:         true,
			})
			require.NoError(t, err)

			require.Equal(t, tt.wantClears, botControl.clearCalls)
			require.Nil(t, findGrantedEvent(outboxStore))

			released := findReleasedEvent(outboxStore)
			if !tt.wantReleased {
				require.Nil(t, released)

				return
			}

			require.NotNil(t, released)
			require.Equal(t, ownerBotMemberID, released.MemberID)
			require.Equal(t, string(model.BotControlReasonAgentTakeover), released.Reason)
			require.NotNil(t, released.Sub)
			require.Equal(t, ownerBotSub, *released.Sub)
		})
	}
}

func TestHandBackToBot(t *testing.T) {
	t.Parallel()

	operatorContactID := uuid.New()
	clientContactID := uuid.New()
	ownerBotMemberID := uuid.New()
	ownerBotContactID := uuid.New()

	members := []*model.ThreadDialogExtended{
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: operatorContactID, ThreadRole: model.RoleMember},
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: clientContactID, ThreadRole: model.RoleOwner},
	}
	ownerBot := &model.BotControlStackEntry{MemberID: &ownerBotMemberID, ContactID: ownerBotContactID, DomainID: 1}

	tests := []struct {
		name        string
		initiator   uuid.UUID
		restore     *model.BotControlStackEntry
		granted     bool
		wantCode    codes.Code
		wantRestore int
		wantGranted bool
	}{
		{name: "operator hands the thread back", initiator: operatorContactID, restore: ownerBot, granted: true, wantRestore: 1, wantGranted: true},
		{name: "bot already holds the thread", initiator: operatorContactID, restore: ownerBot, wantRestore: 1, wantGranted: true},
		{name: "thread without a bot", initiator: operatorContactID, wantCode: codes.FailedPrecondition, wantRestore: 1},
		{name: "client cannot hand back", initiator: clientContactID, wantCode: codes.PermissionDenied},
		{name: "non-member cannot hand back", initiator: uuid.New(), wantCode: codes.PermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			botControl := &fakeBotControlStore{restoreEntry: tt.restore, restoreGranted: tt.granted}
			outboxStore := &fakeOutboxStore{}

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadDialogStore: &fakeThreadDialogStore{fullViewResult: members},
					outboxStore:       outboxStore,
					botControlStore:   botControl,
				},
			}

			err := svc.HandBackToBot(context.Background(), &dto.HandBackToBotRequest{
				ThreadID:           uuid.New(),
				InitiatorContactID: tt.initiator,
				DomainID:           1,
			})

			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, errors.Code(err))
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.wantRestore, botControl.restoreCalls)

			granted := findGrantedEvent(outboxStore)
			if !tt.wantGranted {
				require.Nil(t, granted)

				return
			}

			require.NotNil(t, granted)
			require.Equal(t, ownerBotMemberID, granted.MemberID)
			require.Equal(t, string(model.BotControlReasonAgentHandback), granted.Reason)
			require.True(t, granted.IsResume)
		})
	}
}

func TestRemoveMember_Operator(t *testing.T) {
	t.Parallel()

	ownerBotMemberID := uuid.New()
	ownerBot := &model.BotControlStackEntry{MemberID: &ownerBotMemberID, ContactID: uuid.New(), DomainID: 1}

	tests := []struct {
		name        string
		remaining   []*model.ThreadDialog
		granted     bool
		wantRestore int
		wantGranted bool
	}{
		{
			name:        "last operator leaves",
			remaining:   []*model.ThreadDialog{{BaseModel: shared.BaseModel{ID: ownerBotMemberID}, ThreadRole: model.RoleOwner, IsBot: true}},
			granted:     true,
			wantRestore: 1,
			wantGranted: true,
		},
		{
			name:        "bot already holds the thread",
			remaining:   []*model.ThreadDialog{{BaseModel: shared.BaseModel{ID: ownerBotMemberID}, ThreadRole: model.RoleOwner, IsBot: true}},
			wantRestore: 1,
		},
		{
			name: "another operator stays",
			remaining: []*model.ThreadDialog{
				{BaseModel: shared.BaseModel{ID: ownerBotMemberID}, ThreadRole: model.RoleOwner, IsBot: true},
				{BaseModel: shared.BaseModel{ID: uuid.New()}, ThreadRole: model.RoleMember},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threadID := uuid.New()
			botControl := &fakeBotControlStore{restoreEntry: ownerBot, restoreGranted: tt.granted}
			outboxStore := &fakeOutboxStore{}

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadDialogStore: &fakeThreadDialogStore{
						targetPair: &model.ThreadDialogExtended{
							BaseModel:  shared.BaseModel{ID: uuid.New(), DomainID: 1},
							ContactID:  uuid.New(),
							ThreadID:   threadID,
							ThreadRole: model.RoleMember,
						},
						quickViewResult: tt.remaining,
					},
					messageStore:    &fakeMessageStore{},
					outboxStore:     outboxStore,
					botControlStore: botControl,
				},
			}

			err := svc.RemoveMember(context.Background(), &dto.RemoveMemberRequest{TargetMemberID: uuid.New()})
			require.NoError(t, err)

			require.Equal(t, tt.wantRestore, botControl.restoreCalls)

			granted := findGrantedEvent(outboxStore)
			if !tt.wantGranted {
				require.Nil(t, granted)

				return
			}

			require.NotNil(t, granted)
			require.Equal(t, ownerBotMemberID, granted.MemberID)
			require.Equal(t, string(model.BotControlReasonAgentLeft), granted.Reason)
		})
	}
}

func TestEnsureBotControl_OperatorPresent_KeepsControlReleased(t *testing.T) {
	t.Parallel()

	ownerBotMemberID := uuid.New()
	botControl := &fakeBotControlStore{
		stackResult: []*model.BotControlStackEntry{{MemberID: &ownerBotMemberID}},
	}
	outboxStore := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: &fakeThreadDialogStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
	}

	thread := &model.Thread{
		ID: uuid.New(),
		Members: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: ownerBotMemberID}, ThreadRole: model.RoleOwner, IsBot: true},
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ThreadRole: model.RoleOwner},
			{BaseModel: shared.BaseModel{ID: uuid.New()}, ThreadRole: model.RoleMember},
		},
	}

	require.NoError(t, svc.ensureBotControl(context.Background(), thread, 1))
	require.Equal(t, 0, botControl.setControllerCalls)
	require.Equal(t, 0, botControl.pushCalls)
	require.Nil(t, thread.BotControllerID)
	require.Nil(t, findGrantedEvent(outboxStore))
}

func TestTakeOverFromBot(t *testing.T) {
	t.Parallel()

	operatorContactID := uuid.New()
	clientContactID := uuid.New()
	ownerBotMemberID := uuid.New()
	ownerBotContactID := uuid.New()
	ownerBotSub := int64(77)

	members := []*model.ThreadDialogExtended{
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: operatorContactID, ThreadRole: model.RoleMember},
		{BaseModel: shared.BaseModel{ID: uuid.New(), DomainID: 1}, ContactID: clientContactID, ThreadRole: model.RoleOwner},
	}

	tests := []struct {
		name         string
		initiator    uuid.UUID
		controller   *uuid.UUID
		wantCode     codes.Code
		wantClears   int
		wantReleased bool
	}{
		{name: "operator takes the thread from the bot", initiator: operatorContactID, controller: &ownerBotMemberID, wantClears: 1, wantReleased: true},
		{name: "operator already holds the thread", initiator: operatorContactID, wantClears: 1},
		{name: "client cannot take over", initiator: clientContactID, controller: &ownerBotMemberID, wantCode: codes.PermissionDenied},
		{name: "non-member cannot take over", initiator: uuid.New(), controller: &ownerBotMemberID, wantCode: codes.PermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			botControl := &fakeBotControlStore{clearedMemberID: tt.controller}
			outboxStore := &fakeOutboxStore{}

			svc := &ThreadManagementService{
				uow: fakeUnitOfWork{
					threadDialogStore: &fakeThreadDialogStore{
						fullViewResult: members,
						quickViewResult: []*model.ThreadDialog{
							{BaseModel: shared.BaseModel{ID: ownerBotMemberID}, ContactID: ownerBotContactID, ThreadRole: model.RoleOwner, IsBot: true},
						},
					},
					outboxStore:     outboxStore,
					botControlStore: botControl,
				},
				contactInfo: &fakeContactInfo{subs: map[uuid.UUID]int64{ownerBotContactID: ownerBotSub}},
			}

			err := svc.TakeOverFromBot(context.Background(), &dto.TakeOverFromBotRequest{
				ThreadID:           uuid.New(),
				InitiatorContactID: tt.initiator,
				DomainID:           1,
			})

			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, errors.Code(err))
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.wantClears, botControl.clearCalls)

			released := findReleasedEvent(outboxStore)
			if !tt.wantReleased {
				require.Nil(t, released)

				return
			}

			require.NotNil(t, released)
			require.Equal(t, ownerBotMemberID, released.MemberID)
			require.Equal(t, string(model.BotControlReasonAgentTakeover), released.Reason)
			require.NotNil(t, released.Sub)
			require.Equal(t, ownerBotSub, *released.Sub)
		})
	}
}
