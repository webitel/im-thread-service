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

type adoptBotFixture struct {
	threadID       uuid.UUID
	agentContactID uuid.UUID
	transferBotID  uuid.UUID
	dialogs        *fakeThreadDialogStore
	botControl     *fakeBotControlStore
	outbox         *fakeOutboxStore
	svc            *ThreadManagementService
}

func newAdoptBotFixture(adopted uuid.UUID) *adoptBotFixture {
	f := &adoptBotFixture{
		threadID:       uuid.New(),
		agentContactID: uuid.New(),
		transferBotID:  uuid.New(),
		outbox:         &fakeOutboxStore{},
		botControl:     &fakeBotControlStore{},
	}

	f.dialogs = &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: uuid.New(), DomainID: 1},
				ContactID:   f.agentContactID,
				ThreadID:    f.threadID,
				ThreadRole:  model.RoleMember,
				Permissions: model.ThreadPermissions{CanAddMembers: true},
			},
		},
		quickViewResult: []*model.ThreadDialog{},
		adoptResult:     adopted,
	}

	f.svc = &ThreadManagementService{
		tagHooks: &fakeTagHooks{},
		uow: fakeUnitOfWork{
			threadDialogStore: f.dialogs,
			messageStore:      &fakeMessageStore{},
			outboxStore:       f.outbox,
			botControlStore:   f.botControl,
		},
		privacyChecker: fakePrivacyChecker{},
	}

	return f
}

func (f *adoptBotFixture) transferToBot(t *testing.T) uuid.UUID {
	t.Helper()

	newMemberID, err := f.svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           f.threadID,
		NewMemberContactID: f.transferBotID,
		InitiatorContactID: f.agentContactID,
		NewMemberRole:      model.RoleMember,
		TargetIsBot:        true,
	})
	require.NoError(t, err)

	return newMemberID
}

func TestTransfer_ToBot_AdoptedBotPushedBeforeIncomingBot(t *testing.T) {
	adopted := uuid.New()
	f := newAdoptBotFixture(adopted)

	newMemberID := f.transferToBot(t)

	require.Equal(t, 1, f.dialogs.adoptCalls)
	require.Equal(t, newMemberID, f.dialogs.adoptNewMember)
	require.Len(t, f.botControl.pushTransitions, 2)
	require.Equal(t, adopted, f.botControl.pushTransitions[0].NewMemberID)
	require.Equal(t, model.BotControlReasonInitial, f.botControl.pushTransitions[0].Reason)
	require.Equal(t, newMemberID, f.botControl.pushTransitions[1].NewMemberID)
	require.Equal(t, model.BotControlReasonTransfer, f.botControl.pushTransitions[1].Reason)
}

func TestTransfer_ToBot_NothingAdopted_SinglePush(t *testing.T) {
	f := newAdoptBotFixture(uuid.Nil)

	newMemberID := f.transferToBot(t)

	require.Equal(t, 1, f.dialogs.adoptCalls)
	require.Len(t, f.botControl.pushTransitions, 1)
	require.Equal(t, newMemberID, f.botControl.pushTransitions[0].NewMemberID)
}

func TestAddMember_Bot_AdoptedBotPushedBeforeIncomingBot(t *testing.T) {
	adopted := uuid.New()
	f := newAdoptBotFixture(adopted)

	_, err := f.svc.AddMember(context.Background(), &dto.AddMemberRequest{
		ThreadID:           f.threadID,
		NewMemberContactID: f.transferBotID,
		InitiatorContactID: f.agentContactID,
		NewMemberRole:      model.RoleMember,
		IsBot:              true,
	})
	require.NoError(t, err)

	require.Equal(t, 1, f.dialogs.adoptCalls)
	require.Len(t, f.botControl.pushTransitions, 2)
	require.Equal(t, adopted, f.botControl.pushTransitions[0].NewMemberID)
}

func TestTransfer_ToHumanAgent_DoesNotAdopt(t *testing.T) {
	f := newAdoptBotFixture(uuid.New())

	_, err := f.svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           f.threadID,
		NewMemberContactID: uuid.New(),
		InitiatorContactID: f.agentContactID,
		NewMemberRole:      model.RoleMember,
		TargetIsBot:        false,
	})
	require.NoError(t, err)

	require.Zero(t, f.dialogs.adoptCalls)
	require.Empty(t, f.botControl.pushTransitions)
}

func TestTransfer_ToBot_ImportedThreadWithUntrackedPortalBot(t *testing.T) {
	var (
		threadID       = uuid.MustParse("85a27368-d03f-41e3-8f96-92d64c30c23e")
		portalMemberID = uuid.MustParse("3fda8f79-1655-4830-bd4c-2aa85370d605")
		portalContact  = uuid.MustParse("01a01e29-3ed2-7caf-9056-47c29c95619a")
		agentMemberID  = uuid.MustParse("01a17822-d75f-755f-8cdf-dfba43978c8d")
		agentContact   = uuid.MustParse("01a16202-b7b6-7256-87b8-6cf83bb578a9")
		tier1Contact   = uuid.MustParse("01a04158-06a0-7203-81d5-726641371d72")
	)

	dialogs := &fakeThreadDialogStore{
		fullViewResult: []*model.ThreadDialogExtended{
			{
				BaseModel:   shared.BaseModel{ID: agentMemberID, DomainID: 1},
				ContactID:   agentContact,
				ThreadID:    threadID,
				ThreadRole:  model.RoleMember,
				Permissions: model.ThreadPermissions{CanAddMembers: true},
			},
		},
		quickViewResult: []*model.ThreadDialog{},
		adoptResult:     portalMemberID,
	}
	botControl := &fakeBotControlStore{memberContacts: map[uuid.UUID]uuid.UUID{portalMemberID: portalContact}}
	outbox := &fakeOutboxStore{}

	svc := &ThreadManagementService{
		tagHooks: &fakeTagHooks{},
		uow: fakeUnitOfWork{
			threadDialogStore: dialogs,
			messageStore:      &fakeMessageStore{},
			outboxStore:       outbox,
			botControlStore:   botControl,
		},
		privacyChecker: fakePrivacyChecker{},
		contactInfo: &fakeContactInfo{
			subs: map[uuid.UUID]int64{portalContact: 1, tier1Contact: 145},
		},
	}

	tier1MemberID, err := svc.Transfer(context.Background(), &dto.TransferThreadRequest{
		ThreadID:           threadID,
		NewMemberContactID: tier1Contact,
		InitiatorContactID: agentContact,
		NewMemberRole:      model.RoleMember,
		TargetIsBot:        true,
	})
	require.NoError(t, err)

	require.Len(t, botControl.pushTransitions, 2)
	require.Equal(t, portalMemberID, botControl.pushTransitions[0].NewMemberID, "portal bot becomes the owner at position 0")
	require.Equal(t, tier1MemberID, botControl.pushTransitions[1].NewMemberID, "Tier 1 Level 2 is pushed on top")

	released := findReleasedEvent(outbox)
	require.NotNil(t, released, "portal bot must be released so flow_manager suspends it")
	require.Equal(t, portalMemberID, released.MemberID)
	require.NotNil(t, released.Sub)
	require.EqualValues(t, 1, *released.Sub)

	granted := findGrantedEvent(outbox)
	require.NotNil(t, granted)
	require.Equal(t, tier1MemberID, granted.MemberID)
	require.NotNil(t, granted.Sub)
	require.EqualValues(t, 145, *granted.Sub)
	require.NotNil(t, granted.ReleasedSub, "released_sub makes flow_manager run Tier 1 Level 2 as a nested schema")
	require.EqualValues(t, 1, *granted.ReleasedSub)
	require.Equal(t, 1, granted.Position)
}
