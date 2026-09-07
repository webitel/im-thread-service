package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

// fakeBotControlStore captures Push/Pop calls and returns configurable results.
type fakeBotControlStore struct {
	prevEntry   *model.BotControlStackEntry   // Prev field of BotControlPushResult
	newTopEntry *model.BotControlStackEntry   // returned by Pop (new top after removal)
	stackResult []*model.BotControlStackEntry // returned by GetStack

	clearedMemberID *uuid.UUID // returned by ClearController

	lastPushTransition model.BotControlTransition
	lastPopMemberID    uuid.UUID
	lastPopReason      model.BotControlReason
	popCalls           int
	clearCalls         int

	pushCalls                 int
	setControllerCalls        int
	lastSetControllerMemberID uuid.UUID
}

func (f *fakeBotControlStore) Push(_ context.Context, transition model.BotControlTransition) (*model.BotControlPushResult, error) {
	f.pushCalls++
	f.lastPushTransition = transition

	return &model.BotControlPushResult{Prev: f.prevEntry}, nil
}

func (f *fakeBotControlStore) Pop(_ context.Context, _, memberID uuid.UUID, reason model.BotControlReason, _ *uuid.UUID) (*model.BotControlStackEntry, error) {
	f.popCalls++
	f.lastPopMemberID = memberID
	f.lastPopReason = reason

	return f.newTopEntry, nil
}

func (f *fakeBotControlStore) GetStack(_ context.Context, _ uuid.UUID) ([]*model.BotControlStackEntry, error) {
	return f.stackResult, nil
}

func (f *fakeBotControlStore) ClearController(_ context.Context, _ uuid.UUID) (*uuid.UUID, error) {
	f.clearCalls++

	return f.clearedMemberID, nil
}

func (f *fakeBotControlStore) SetController(_ context.Context, _, memberID uuid.UUID) error {
	f.setControllerCalls++
	f.lastSetControllerMemberID = memberID

	return nil
}

var _ store.BotControlStore = (*fakeBotControlStore)(nil)

// findGrantedEvent returns the first BotControlGranted event from outbox, nil if absent.
func findGrantedEvent(outbox *fakeOutboxStore) *event.BotControlGranted {
	for _, pub := range outbox.published {
		if e, ok := pub.event.(*event.BotControlGranted); ok {
			return e
		}
	}

	return nil
}

// findReleasedEvent returns the first BotControlReleased event from outbox, nil if absent.
func findReleasedEvent(outbox *fakeOutboxStore) *event.BotControlReleased {
	for _, pub := range outbox.published {
		if e, ok := pub.event.(*event.BotControlReleased); ok {
			return e
		}
	}

	return nil
}

func TestAddMember_BotContact_PushesStackAndPublishesBotControlEvents(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	initiatorMemberID := uuid.New()
	botContactID := uuid.New()

	prevMemberID := uuid.New()
	prevEntry := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &prevMemberID, Position: 0,
	}

	botControl := &fakeBotControlStore{prevEntry: prevEntry}
	outboxStore := &fakeOutboxStore{}

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:   initiatorContactID,
				ThreadID:    threadID,
				ThreadRole:  model.RoleOwner,
				Permissions: model.ThreadPermissions{CanAddMembers: true},
				Settings:    model.BaseThreadSetting{Title: "Test"},
			},
		},
		quickViewResult: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: initiatorMemberID}, ContactID: initiatorContactID, ThreadID: threadID},
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
		privacyChecker: fakePrivacyChecker{},
	}

	_, err := svc.AddMember(context.Background(), &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: botContactID,
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
		IsBot:              true,
	})

	require.NoError(t, err)

	// dialog created with correct bot flags
	require.NotNil(t, threadDialogStore.lastCreate)
	require.True(t, threadDialogStore.lastCreate.IsBot)
	require.True(t, threadDialogStore.lastCreate.AutoLeave, "bot added by operator must auto-leave by default")

	// Push called with correct transition
	require.Equal(t, threadID, botControl.lastPushTransition.ThreadID)
	require.Equal(t, model.BotControlReasonTransfer, botControl.lastPushTransition.Reason)

	// BotControlGranted published for new bot
	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted, "BotControlGranted must be published for new bot")
	require.Equal(t, string(model.BotControlReasonTransfer), granted.Reason)
	require.False(t, granted.IsResume, "newly added bot starts fresh, not a resume")
	require.Equal(t, prevEntry.Position+1, granted.Position)
}

func TestAddMember_BotContact_AutoLeaveFalse_WhenExplicitlySet(t *testing.T) {
	threadID := uuid.New()
	initiatorMemberID := uuid.New()
	initiatorContactID := uuid.New()

	isTemp := false
	botControl := &fakeBotControlStore{}

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:   initiatorContactID,
				ThreadID:    threadID,
				ThreadRole:  model.RoleOwner,
				Permissions: model.ThreadPermissions{CanAddMembers: true},
			},
		},
		quickViewResult: []*model.ThreadDialog{},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       &fakeOutboxStore{},
			botControlStore:   botControl,
		},
		privacyChecker: fakePrivacyChecker{},
	}

	_, err := svc.AddMember(context.Background(), &dto.AddMemberRequest{
		ThreadID:           threadID,
		NewMemberContactID: uuid.New(),
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
		IsBot:              true,
		AutoLeave:          &isTemp,
	})

	require.NoError(t, err)
	require.NotNil(t, threadDialogStore.lastCreate)
	require.False(t, threadDialogStore.lastCreate.AutoLeave, "explicit auto_leave=false must be respected")
}

func TestTransfer_BotContact_PushesStackAndPublishesBotControlEvents(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	initiatorMemberID := uuid.New()
	botContactID := uuid.New()

	prevMemberID := uuid.New()
	prevEntry := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &prevMemberID, Position: 0,
	}

	botControl := &fakeBotControlStore{prevEntry: prevEntry}
	outboxStore := &fakeOutboxStore{}

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:   initiatorContactID,
				ThreadID:    threadID,
				ThreadRole:  model.RoleOwner,
				Permissions: model.ThreadPermissions{CanAddMembers: true},
				Settings:    model.BaseThreadSetting{Title: "Test"},
			},
		},
		quickViewResult: []*model.ThreadDialog{},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
		privacyChecker: fakePrivacyChecker{},
	}

	newMemberID, err := svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           threadID,
		NewMemberContactID: botContactID,
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
		TargetIsBot:        true,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, newMemberID)

	require.Equal(t, threadID, botControl.lastPushTransition.ThreadID)
	require.Equal(t, model.BotControlReasonTransfer, botControl.lastPushTransition.Reason)

	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted)
	require.False(t, granted.IsResume)
}

// A bot handing the conversation off to a HUMAN agent must fully release bot control:
// Pop is called with reason=handoff and NO bot.control.granted is published — otherwise the
// owner bot would be re-woken and keep re-triggering (e.g. re-transferring) on every message
// while the agent handles the thread.
func TestTransfer_ToHumanAgent_ReleasesBotControlWithoutGrant(t *testing.T) {
	threadID := uuid.New()
	initiatorContactID := uuid.New()
	initiatorMemberID := uuid.New()
	agentContactID := uuid.New()

	// Pop returns nil (control released, no new controller) — the handoff case.
	botControl := &fakeBotControlStore{}
	outboxStore := &fakeOutboxStore{}

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: initiatorMemberID, DomainID: 1},
				ContactID:   initiatorContactID,
				ThreadID:    threadID,
				ThreadRole:  model.RoleOwner,
				IsBot:       true, // initiator is the (owner) bot transferring out
				Permissions: model.ThreadPermissions{CanAddMembers: true},
				Settings:    model.BaseThreadSetting{Title: "Test"},
			},
		},
		quickViewResult: []*model.ThreadDialog{},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
		},
		privacyChecker: fakePrivacyChecker{},
	}

	_, err := svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           threadID,
		NewMemberContactID: agentContactID,
		InitiatorContactID: initiatorContactID,
		NewMemberRole:      model.RoleMember,
		TargetIsBot:        false, // handing off to a human agent
	})
	require.NoError(t, err)

	require.Equal(t, model.BotControlReasonHandoff, botControl.lastPopReason,
		"handoff to a human agent must pop with reason=handoff")

	require.Nil(t, findGrantedEvent(outboxStore),
		"no bot.control.granted must be published when handing off to a human agent")
}

func TestRemoveMember_ActiveBot_PopsStackAndPublishesBotControlGrantedWithIsResume(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()
	botContactID := uuid.New()
	newTopMemberID := uuid.New()
	newTopContactID := uuid.New()

	newTop := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &newTopMemberID, Position: 0,
	}
	botControl := &fakeBotControlStore{newTopEntry: newTop}
	outboxStore := &fakeOutboxStore{}

	threadDialogStore := &fakeThreadDialogStore{
		targetPair: &model.ThreadDialogExtended{
			BaseModel:  shared.BaseModel{ID: botMemberID, DomainID: 1},
			ContactID:  botContactID,
			ThreadID:   threadID,
			ThreadRole: model.RoleMember,
			IsBot:      true,
			AutoLeave:  true,
		},
		quickViewResult: []*model.ThreadDialog{},
		// GetFullView returns new top dialog for BotControlGranted
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

	err := svc.RemoveMember(context.Background(), &dto.RemoveMemberRequest{
		TargetMemberID:     botMemberID,
		InitiatorContactID: uuid.Nil,
	})

	require.NoError(t, err)

	// Pop called with correct params
	require.Equal(t, botMemberID, botControl.lastPopMemberID)
	require.Equal(t, model.BotControlReasonRemoved, botControl.lastPopReason)

	// BotControlGranted published with is_resume=true for new top
	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted, "BotControlGranted must be published when returning control")
	require.Equal(t, newTopMemberID, granted.MemberID)
	require.True(t, granted.IsResume, "returning control to previous bot must set is_resume=true")
	require.Equal(t, newTop.Position, granted.Position)
}

func TestCompleteBotControl_PopsStackAndPublishesGrantedWithIsResume(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()
	newTopMemberID := uuid.New()
	newTopContactID := uuid.New()

	newTop := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &newTopMemberID, Position: 0,
	}
	botControl := &fakeBotControlStore{
		newTopEntry: newTop,
		// GetStack must return botMemberID as the top so the active-controller check passes.
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

	mockThreadStore := &fakeThreadStore{
		getResult: &model.Thread{
			ID:         threadID,
			OwnerBotID: nil, // Перевірка ownerBotID != req.MemberID пройде успішно
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
			threadStore:       mockThreadStore,
		},
	}

	err := svc.CompleteBotControl(context.Background(), &dto.CompleteBotControlRequest{
		ThreadID: threadID,
		MemberID: botMemberID,
		DomainID: 1,
	})

	require.NoError(t, err)

	// Pop called with completed reason
	require.Equal(t, botMemberID, botControl.lastPopMemberID)
	require.Equal(t, model.BotControlReasonCompleted, botControl.lastPopReason)

	// BotControlGranted with is_resume=true for new top
	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted)
	require.Equal(t, newTopMemberID, granted.MemberID)
	require.True(t, granted.IsResume)
	require.Equal(t, newTop.Position, granted.Position)
}

// TestCompleteBotControl_TransientToOwner_NoGrant_KeepsController covers the regression fix:
// when a transient (auto_leave) bot completes and control falls back to the OWNER bot, no
// BotControlGranted must be published and the controller must NOT be cleared. Pop leaves
// bot_controller_id pointing at the owner, so the next inbound customer message is routed to
// it and flow_manager starts the owner schema from scratch via nodeMessage — instead of the
// owner speaking the instant the transient bot leaves.
func TestCompleteBotControl_TransientToOwner_NoGrant_KeepsController(t *testing.T) {
	threadID := uuid.New()
	transientMemberID := uuid.New()
	ownerBotMemberID := uuid.New()

	// After popping the transient, control falls back to the owner bot.
	newTop := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &ownerBotMemberID, Position: 0,
	}
	botControl := &fakeBotControlStore{
		newTopEntry: newTop,
		// GetStack returns the transient as the active controller so the check passes.
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &transientMemberID, Position: 1},
		},
	}
	outboxStore := &fakeOutboxStore{}

	threadDialogStore := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel: shared.BaseModel{ID: ownerBotMemberID, DomainID: 1},
				ThreadID:  threadID,
			},
		},
	}

	mockThreadStore := &fakeThreadStore{
		getResult: &model.Thread{
			ID:         threadID,
			OwnerBotID: &ownerBotMemberID, // control returns to the owner bot
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: threadDialogStore,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
			threadStore:       mockThreadStore,
		},
	}

	err := svc.CompleteBotControl(context.Background(), &dto.CompleteBotControlRequest{
		ThreadID: threadID,
		MemberID: transientMemberID,
		DomainID: 1,
	})

	require.NoError(t, err)

	// Transient was popped with the completed reason.
	require.Equal(t, transientMemberID, botControl.lastPopMemberID)
	require.Equal(t, model.BotControlReasonCompleted, botControl.lastPopReason)

	// No grant: the owner must wake on the next message, not on completion.
	require.Nil(t, findGrantedEvent(outboxStore),
		"no BotControlGranted when control returns to the owner bot")

	// Controller kept (not cleared) so the message routes to the owner.
	require.Zero(t, botControl.clearCalls,
		"controller must stay pointing at the owner, not be cleared to NULL")
}

func TestCompleteBotControl_EmptyStack_OnlyPublishesReleased(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()

	// Pop returns nil = stack becomes empty after this bot completes.
	// GetStack must still return botMemberID as the current top before Pop.
	botControl := &fakeBotControlStore{
		newTopEntry: nil,
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &botMemberID, Position: 0},
		},
	}
	outboxStore := &fakeOutboxStore{}

	mockThreadStore := &fakeThreadStore{
		getResult: &model.Thread{
			ID:         threadID,
			OwnerBotID: nil,
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: &fakeThreadDialogStore{},
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
			threadStore:       mockThreadStore,
		},
	}

	err := svc.CompleteBotControl(context.Background(), &dto.CompleteBotControlRequest{
		ThreadID: threadID,
		MemberID: botMemberID,
		DomainID: 1,
	})

	require.NoError(t, err)

	granted := findGrantedEvent(outboxStore)
	require.Nil(t, granted, "no BotControlGranted when stack is empty")
}

// TestReleaseBotControl_SingleBot_PublishesReleasedOnly covers /close on a single-bot thread:
// the stack empties (Pop returns nil), so only a released(client_leave) event fires and control
// is fully dropped (no granted). This is the state that lets ensureBotControl re-arm /close later.
func TestReleaseBotControl_SingleBot_PublishesReleasedOnly(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()
	botContactID := uuid.New()
	initiatorMemberID := uuid.New()

	// Pop returns nil = /close empties the stack (single-bot direct thread).
	botControl := &fakeBotControlStore{
		newTopEntry: nil,
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &botMemberID, ContactID: botContactID, Position: 0},
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

	// Pop invoked once for the active bot with client_leave reason.
	require.Equal(t, 1, botControl.popCalls)
	require.Equal(t, botMemberID, botControl.lastPopMemberID)
	require.Equal(t, model.BotControlReasonClientLeave, botControl.lastPopReason)

	// released published with client_leave and no next member.
	released := findReleasedEvent(outboxStore)
	require.NotNil(t, released, "BotControlReleased must be published on /close")
	require.Equal(t, string(model.BotControlReasonClientLeave), released.Reason)
	require.Equal(t, botMemberID, released.MemberID)
	require.Nil(t, released.NextMemberID, "no next controller when the last bot is closed")

	// No granted — the bot is fully stopped, not handed back to the owner bot.
	require.Nil(t, findGrantedEvent(outboxStore), "no BotControlGranted when the last bot is closed")
}

// TestReleaseBotControl_BotBelow_ReturnsControlWithGranted covers /close when another bot remains
// below the active one on the stack: control transitions to that bot (granted, is_resume=true),
// which is the pre-existing multi-bot behavior and must be preserved.
func TestReleaseBotControl_BotBelow_ReturnsControlWithGranted(t *testing.T) {
	threadID := uuid.New()
	botMemberID := uuid.New()
	botContactID := uuid.New()
	newTopMemberID := uuid.New()
	newTopContactID := uuid.New()

	newTop := &model.BotControlStackEntry{
		ID: uuid.New(), ThreadID: threadID, MemberID: &newTopMemberID, ContactID: newTopContactID, Position: 0,
	}
	botControl := &fakeBotControlStore{
		newTopEntry: newTop,
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &newTopMemberID, Position: 0},
			{MemberID: &botMemberID, ContactID: botContactID, Position: 1},
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
		ThreadID: threadID,
		DomainID: 1,
	})
	require.NoError(t, err)

	// Pop targets the active (top) bot with client_leave.
	require.Equal(t, botMemberID, botControl.lastPopMemberID)
	require.Equal(t, model.BotControlReasonClientLeave, botControl.lastPopReason)

	released := findReleasedEvent(outboxStore)
	require.NotNil(t, released)
	require.NotNil(t, released.NextMemberID)
	require.Equal(t, newTopMemberID, *released.NextMemberID)

	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted, "control returns to the bot below on /close")
	require.Equal(t, newTopMemberID, granted.MemberID)
	require.True(t, granted.IsResume)
}

// TestReleaseBotControl_EmptyStack_NoOp verifies /close is idempotent: with no active bot on the
// stack it does nothing — no Pop, no released, no granted.
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
	require.NoError(t, err)

	require.Equal(t, 0, botControl.popCalls, "no Pop when there is no active bot")
	require.Nil(t, findReleasedEvent(outboxStore))
	require.Nil(t, findGrantedEvent(outboxStore))
}

// TestReleaseBotControl_DivergedState_ClearsControllerAndPublishesReleased covers the case where
// the stack is empty but the thread still has a bot_controller_id (legacy data / owner-bot
// fallback): /close must clear it and publish released so the running schema is stopped.
func TestReleaseBotControl_DivergedState_ClearsControllerAndPublishesReleased(t *testing.T) {
	threadID := uuid.New()
	controllerID := uuid.New()

	botControl := &fakeBotControlStore{
		stackResult:     nil,           // stack empty
		clearedMemberID: &controllerID, // ...but a controller lingers on the thread
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
		ThreadID: threadID,
		DomainID: 1,
	})
	require.NoError(t, err)

	require.Equal(t, 1, botControl.clearCalls, "diverged state must clear the lingering controller")
	require.Equal(t, 0, botControl.popCalls, "diverged path must not Pop")

	released := findReleasedEvent(outboxStore)
	require.NotNil(t, released, "released must fire so the running schema is stopped")
	require.Equal(t, controllerID, released.MemberID)
	require.Equal(t, string(model.BotControlReasonClientLeave), released.Reason)
	require.Nil(t, released.NextMemberID)

	require.Nil(t, findGrantedEvent(outboxStore), "nothing to grant when clearing a diverged controller")
}

// TestCompleteBotControl_OwnerBot_Forbidden covers the owner bot trying to complete/leave bot
// control. The owner is the permanent controller and must never leave: CompleteBotControl must
// reject it with a Forbidden error and leave everything intact — controller NOT cleared, stack
// NOT popped, no granted event. Keeping bot_controller_id pointing at the owner is what lets the
// next customer message wake it again (delivery routes to the active controller). Clearing it to
// NULL (the previous behavior) made delivery treat the thread as "no active bot" so the owner
// was never woken.
func TestCompleteBotControl_OwnerBot_Forbidden(t *testing.T) {
	threadID := uuid.New()
	ownerMemberID := uuid.New()

	botControl := &fakeBotControlStore{
		// Owner is the sole controller: it is the top of the stack.
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &ownerMemberID, Position: 0},
		},
	}
	outboxStore := &fakeOutboxStore{}

	mockThreadStore := &fakeThreadStore{
		getResult: &model.Thread{
			ID:         threadID,
			OwnerBotID: &ownerMemberID, // the completing member IS the owner bot
		},
	}

	svc := &ThreadManagementService{
		uow: fakeUnitOfWork{
			threadDialogStore: &fakeThreadDialogStore{},
			messageStore:      &fakeMessageStore{},
			outboxStore:       outboxStore,
			botControlStore:   botControl,
			threadStore:       mockThreadStore,
		},
	}

	err := svc.CompleteBotControl(context.Background(), &dto.CompleteBotControlRequest{
		ThreadID: threadID,
		MemberID: ownerMemberID,
		DomainID: 1,
	})

	require.Error(t, err, "owner bot completion must be forbidden")
	require.Equal(t, 0, botControl.clearCalls, "owner completion must NOT clear the controller")
	require.Equal(t, 0, botControl.popCalls, "owner must never be popped from the stack")
	require.Nil(t, findGrantedEvent(outboxStore), "no granted on a rejected owner completion")
}

// TestEnsureBotControl_ExistingStack_ReGrantsWithoutPush covers the restart path: a thread whose
// owner bot is still on the stack but idle (bot_controller_id was cleared). ensureBotControl must
// re-point the controller at the existing top via SetController (NOT push a duplicate entry) and
// publish a granted event so flow_manager starts a fresh schema.
func TestEnsureBotControl_ExistingStack_ReGrantsWithoutPush(t *testing.T) {
	threadID := uuid.New()
	ownerMemberID := uuid.New()
	ownerContactID := uuid.New()

	botControl := &fakeBotControlStore{
		// Owner still on the stack — the idle state left by CompleteBotControl.
		stackResult: []*model.BotControlStackEntry{
			{MemberID: &ownerMemberID, Position: 0},
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

	thread := &model.Thread{
		ID: threadID,
		Members: []*model.ThreadDialog{
			{BaseModel: shared.BaseModel{ID: ownerMemberID, DomainID: 1}, ContactID: ownerContactID, IsBot: true},
		},
	}

	err := svc.ensureBotControl(context.Background(), thread, 1)

	require.NoError(t, err)
	require.Equal(t, 1, botControl.setControllerCalls, "must re-point controller at existing top")
	require.Equal(t, ownerMemberID, botControl.lastSetControllerMemberID)
	require.Equal(t, 0, botControl.pushCalls, "must not push a duplicate entry for the owner already on the stack")

	granted := findGrantedEvent(outboxStore)
	require.NotNil(t, granted, "granted must fire so flow_manager restarts the schema")
	require.Equal(t, ownerMemberID, granted.MemberID)
	require.False(t, granted.IsResume, "a fresh restart is an initial grant, not a resume")
}
