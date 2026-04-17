package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

var (
	notThreadMemberError = errors.New("member is not part of thread")
)

type (
	ThreadManagementService struct {
		uow            store.UnitOfWork
		logger         *slog.Logger
		privacyChecker ThreadPrivacyChecker
	}

	ThreadPrivacyChecker interface {
		CanInvite(ctx context.Context, initiatorID, targetID uuid.UUID) error
		CanSend(ctx context.Context, senderID, recipientID uuid.UUID) error
	}

	ThreadEvent interface {
		Topic() string
		MustBeThreadEvent()
		event.Outboxer
	}
)

// NewThreadService returns a new thread manager, given a unit of work.
func NewThreadService(logger *slog.Logger, uow store.UnitOfWork, privacyChecker ThreadPrivacyChecker) *ThreadManagementService {
	log := logger.With(slog.String("component", "thread"))

	return &ThreadManagementService{
		uow:            uow,
		logger:         log,
		privacyChecker: privacyChecker,
	}
}

func (t *ThreadManagementService) Search(ctx context.Context, searchRequest *dto.ThreadSearchRequest) ([]*model.Thread, error) {
	if searchRequest == nil {
		return nil, errors.New("search request cannot be nil")
	}
	query := queryobject.NewThreadQueryObject().
		WithSubject().
		WithFields(searchRequest.Fields).
		WithIDFilter(searchRequest.Ids...).
		WithDomainIDFilter(searchRequest.DomainIDs...).
		WithKindFilter(searchRequest.Kinds...).
		WithContactIDFilter(searchRequest.ContactIDs...).
		WithOwnerFilter(searchRequest.Owners...).
		WithSubjectFilter(searchRequest.Q).
		WithLimit(searchRequest.Size).
		WithSort(searchRequest.Sort).
		WithOffset(searchRequest.Page)

	threads, err := t.uow.ThreadStore().Search(ctx, query)
	if err != nil {
		return nil, err
	}

	return threads, nil
}

func (t *ThreadManagementService) findAddMemberActors(ctx context.Context, threadID uuid.UUID, initiatorContactID, targetContactID uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error) {
	actionActors, err := t.uow.ThreadDialogStore().GetFullView(ctx, &model.ThreadDialogStoreFilter{
		ThreadIDs:  []uuid.UUID{threadID},
		ContactIDs: []uuid.UUID{initiatorContactID, targetContactID},
	})
	if err != nil {
		return nil, nil, err
	}

	var initiatorActor, targetActor *model.ThreadDialogExtended
	for _, actor := range actionActors {
		if initiatorActor != nil && targetActor != nil {
			break
		}
		if actor.MemberID == initiatorContactID {
			initiatorActor = actor
		}
		if actor.MemberID == targetContactID {
			targetActor = actor
		}
	}

	return initiatorActor, targetActor, nil
}

func (t *ThreadManagementService) AddMember(ctx context.Context, req *dto.AddMemberRequest) (uuid.UUID, error) {
	if req == nil {
		return uuid.Nil, errors.New("add member request cannot be nil")
	}
	initiator, target, err := t.findAddMemberActors(ctx, req.ThreadID, req.InitiatorContactID, req.NewMemberContactID)
	if err != nil {
		return uuid.Nil, err
	}
	if target != nil {
		return uuid.Nil, errors.NotFound("target member is already part of the thread")
	}

	var (
		titleForTarget = "New thread"
	)
	if req.InitiatorContactID != uuid.Nil {
		err = t.verifyAddMember(ctx, initiator, req.NewMemberContactID, req.NewMemberRole)
		if err != nil {
			return uuid.Nil, err
		}
		titleForTarget = initiator.Settings.Title
	}

	rolePermissions, err := getDefaultPermissionsByRole(req.NewMemberRole)
	if err != nil {
		return uuid.Nil, err
	}
	var createdThreadDialog *model.ThreadDialogExtended
	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		createdThreadDialog, err = t.uow.ThreadDialogStore().Create(ctx, &model.ThreadDialogExtended{
			BaseModel: shared.BaseModel{
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			ThreadID:    req.ThreadID,
			MemberID:    req.NewMemberContactID,
			ThreadRole:  req.NewMemberRole,
			Permissions: *rolePermissions,
			Settings: model.BaseThreadSetting{
				Title: titleForTarget,
			},
		})
		if err != nil {
			return err
		}

		// TODO: optimize publishing, so we can reduce round trips to the store
		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ContactIDs: []uuid.UUID{req.ThreadID},
		})
		if err != nil {
			return err
		}
		eventTemplate := &event.MemberAddedV1{
			ThreadID:        req.ThreadID,
			MemberContactID: createdThreadDialog.MemberID,
			NewMemberID:     createdThreadDialog.ID,
		}
		if initiator != nil {
			eventTemplate.InitiatorMemberID = initiator.ID
			eventTemplate.InitiatorContactID = initiator.MemberID
		}

		for _, receiver := range eventReceivers {
			eventTemplate.Recipient = receiver.ContactID
			err = uow.Outbox().Publish(ctx, eventTemplate.Topic(), eventTemplate)
			if err != nil {
				return err
			}
		}
		return nil

	})
	if err != nil {
		return uuid.Nil, err
	}
	return createdThreadDialog.ID, nil
}
func (t *ThreadManagementService) verifyAddMember(ctx context.Context, initiator *model.ThreadDialogExtended, targetContactID uuid.UUID, role model.ThreadRole) error {
	if initiator == nil {
		return errors.NotFound("thread not found or initiator does not have permission", errors.WithCause(notThreadMemberError))
	}
	if initiator.MemberID == targetContactID {
		return errors.InvalidArgument("cannot add self as member")
	}
	err := t.verifyAddMemberInitiatorPermissions(initiator.ThreadRole, role, &initiator.Permissions)
	if err != nil {
		return err
	}
	err = t.verifyAddMemberTargetPrivacy(ctx, initiator.MemberID, targetContactID)
	if err != nil {
		return err
	}

	return nil
}

func (t *ThreadManagementService) verifyAddMemberInitiatorPermissions(initiatorRole model.ThreadRole, targetRole model.ThreadRole, initiatorPermissions *model.ThreadPermissions) error {
	if initiatorPermissions == nil {
		return errors.Internal("permissions cannot be nil")
	}
	if !initiatorPermissions.CanAddMembers {
		return errors.Forbidden("initiator does not have permission to invite members")
	}
	if initiatorRole <= targetRole {
		return errors.Forbidden("initiator does not have permission to invite a member with same or higher role")
	}

	return nil
}
func (t *ThreadManagementService) verifyAddMemberTargetPrivacy(ctx context.Context, initiatorContactID, targetContactID uuid.UUID) error {
	err := t.privacyChecker.CanInvite(ctx, initiatorContactID, targetContactID)
	if err != nil {
		return err
	}
	return nil
}

func (t *ThreadManagementService) RemoveMember(ctx context.Context, req *dto.RemoveMemberRequest) error {
	if req == nil {
		return errors.New("remove member request cannot be nil")
	}
	initiator, target, err := t.uow.ThreadDialogStore().FindActorsPair(ctx, req.InitiatorContactID, req.TargetMemberID)
	if err != nil {
		return err
	}
	if target == nil {
		return errors.NotFound("target not found")
	}
	if req.InitiatorContactID != uuid.Nil {
		err = t.verifyRemoveMember(initiator, target)
		if err != nil {
			return err
		}
	}

	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		err = uow.ThreadDialogStore().Delete(ctx, target.ID)
		if err != nil {
			return err
		}
		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ContactIDs: []uuid.UUID{target.ThreadID},
		})
		if err != nil {
			return err
		}
		eventTemplate := &event.MemberRemovedV1{
			ThreadID:        target.ThreadID,
			RemovedMemberID: target.ID,
		}
		if initiator != nil {
			eventTemplate.InitiatorMemberID = initiator.ID
			eventTemplate.InitiatorContactID = initiator.MemberID
		}
		for _, receiver := range eventReceivers {
			eventTemplate.Recipient = receiver.ContactID
			err = uow.Outbox().Publish(ctx, eventTemplate.Topic(), eventTemplate)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (t *ThreadManagementService) verifyRemoveMember(initiator *model.ThreadDialogExtended, target *model.ThreadDialogExtended) error {
	if initiator == nil {
		return errors.NotFound("thread not found or initiator does not have permission", errors.WithCause(notThreadMemberError))
	}
	if target == nil {
		return errors.NotFound("target not found")
	}
	if initiator.ID == target.ID {
		return nil
	}
	err := t.verifyRemoveMemberInitiatorPermissions(initiator.ThreadRole, target.ThreadRole, &initiator.Permissions)
	if err != nil {
		return err
	}
	return nil
}

func (t *ThreadManagementService) verifyRemoveMemberInitiatorPermissions(initiatorRole model.ThreadRole, targetRole model.ThreadRole, initiatorPermissions *model.ThreadPermissions) error {
	if initiatorPermissions == nil {
		return errors.New("permissions cannot be nil")
	}
	if !initiatorPermissions.CanRemoveMembers {
		return errors.New("initiator does not have permission to remove members")
	}
	if initiatorRole <= targetRole {
		return errors.New("initiator does not have permission to remove a member with same or higher role")
	}

	return nil
}

func (t *ThreadManagementService) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_manager.ensure_direct_thread"))
	}

	directThread, err := t.searchDirectThread(ctx, req.From, req.To)
	if err != nil {
		return nil, err
	}

	if directThread != nil {
		return directThread, nil
	}

	directThread, err = t.orchestrateDirectThreadCreation(ctx, req)
	if err != nil {
		return nil, err
	}

	return directThread, nil
}

func (t *ThreadManagementService) searchDirectThread(ctx context.Context, from, to *shared.Peer) (*model.Thread, error) {
	if from == nil {
		return nil, errors.InvalidArgument("from cant be null", errors.WithID("service.thread_manager.search_direct_thread"))
	}

	if to == nil {
		return nil, errors.InvalidArgument("to cant be null", errors.WithID("service.thread_manager.search_direct_thread"))
	}

	switch to.Type {
	case shared.PeerContact:
		thread, err := t.uow.ThreadStore().ResolveDirect(ctx, from.ID, to.ID)
		if err != nil {
			return nil, err
		}
		return thread, nil
	case shared.PeerThread:
		threads, err := t.uow.ThreadStore().Search(
			ctx,
			queryobject.NewThreadQueryObject().
				WithIDFilter(to.ID).
				WithContactIDFilter(from.ID).
				WithKindFilter(model.ThreadDirect).
				WithFields([]string{"id", "domain_id", "created_at", "updated_at", "kind", "owner", "subject", "description", "members"}).
				WithLimit(1),
		)
		if err != nil {
			return nil, err
		}
		if len(threads) == 0 {
			return nil, errors.Forbidden("can`t send message to thread not beeing part of", errors.WithID("service.thread_manager.search_direct_thread"))
		}
		return threads[0], nil
	default:
		return nil, errors.New("invalid peer type for direct")
	}
}

func (t *ThreadManagementService) orchestrateDirectThreadCreation(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	err := t.privacyChecker.CanSend(ctx, req.From.ID, req.To.ID)
	if err != nil {
		return nil, err
	}

	var createdThread *model.Thread
	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		savedThread, err := t.createDirectThread(ctx, uow, req.DomainID, req.From, req.To)
		if err != nil {
			return err
		}
		createdThread = savedThread

		members, err := t.initializeDirectThreadDialogs(ctx, uow, createdThread.ID, req.DomainID, req.From, req.To)
		if err != nil {
			return err
		}

		for _, member := range members {
			createdThread.Members = append(createdThread.Members, extendedThreadDialogToSimpleMapper(member))
		}

		events, err := t.buildDirectThreadCreatedEvents(createdThread, req.From, req.To)
		if err != nil {
			return err
		}

		if err = t.publishThreadCreatedEvents(ctx, uow, events...); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return createdThread, nil
}

func (t *ThreadManagementService) createDirectThread(ctx context.Context, uow store.UnitOfWork, domainID int, from, to *shared.Peer) (*model.Thread, error) {
	if from == nil || to == nil {
		return nil, errors.New("from and to peers cannot be nil")
	}
	if uow == nil {
		return nil, errors.New("unit of work cannot be nil")
	}
	var (
		now          = time.Now().UTC()
		directThread = &model.Thread{
			DomainID:  domainID,
			CreatedAt: now,
			UpdatedAt: now,
			Kind:      model.ThreadDirect,
		}
	)
	createdThread, err := uow.ThreadStore().Create(ctx, directThread)
	if err != nil {
		return nil, err
	}
	return createdThread, nil
}

func extendedThreadDialogToSimpleMapper(tde *model.ThreadDialogExtended) *model.ThreadDialog {
	return &model.ThreadDialog{
		BaseModel:  shared.BaseModel{ID: tde.ID},
		ContactID:  tde.MemberID,
		ThreadID:   tde.ThreadID,
		ThreadRole: tde.ThreadRole,
	}
}

func (t *ThreadManagementService) initializeDirectThreadDialogs(ctx context.Context, uow store.UnitOfWork, threadID uuid.UUID, domainID int, from, to *shared.Peer) ([]*model.ThreadDialogExtended, error) {
	if from == nil || to == nil {
		return nil, errors.New("from and to peers cannot be nil")
	}
	if uow == nil {
		return nil, errors.New("unit of work cannot be nil")
	}
	if to.Identity == nil || from.Identity == nil {
		return nil, errors.New("from and to peers must have identity")
	}
	if to.Identity.Name == "" || from.Identity.Name == "" {
		return nil, errors.New("from and to peers must have identity with non empty name")
	}

	initiatorRole := model.RoleOwner
	peerRole := model.RoleOwner
	initiatorPermissions, err := getDefaultPermissionsByRole(initiatorRole)
	if err != nil {
		return nil, err
	}
	targetPermissions, err := getDefaultPermissionsByRole(peerRole)
	if err != nil {
		return nil, err
	}

	var (
		baseModel = shared.BaseModel{
			DomainID: domainID,
		}

		initiatorThreadDialog = &model.ThreadDialogExtended{
			BaseModel:   baseModel,
			ThreadID:    threadID,
			MemberID:    from.ID,
			ThreadRole:  initiatorRole,
			DirectTo:    &to.ID,
			Permissions: *initiatorPermissions,
			Settings: model.BaseThreadSetting{
				Title: to.Identity.Name,
			},
		}
		targerPermissions = &model.ThreadDialogExtended{
			BaseModel:   baseModel,
			ThreadID:    threadID,
			MemberID:    to.ID,
			ThreadRole:  peerRole,
			DirectTo:    &from.ID,
			Permissions: *targetPermissions,
			Settings: model.BaseThreadSetting{
				Title: from.Identity.Name,
			},
		}
	)

	initiatorCreatedThreadDialog, err := uow.ThreadDialogStore().Create(ctx, initiatorThreadDialog)
	if err != nil {
		return nil, err
	}
	targetCreatedThreadDialog, err := uow.ThreadDialogStore().Create(ctx, targerPermissions)
	if err != nil {
		return nil, err
	}

	return []*model.ThreadDialogExtended{initiatorCreatedThreadDialog, targetCreatedThreadDialog}, nil
}

func (t *ThreadManagementService) buildDirectThreadCreatedEvents(thread *model.Thread, from, to *shared.Peer) ([]ThreadEvent, error) {
	if thread == nil || from == nil || to == nil {
		return nil, errors.InvalidArgument("thread, from and to cannot be nil")
	}

	members := make([]*event.ThreadMember, 0, len(thread.Members))
	for _, member := range thread.Members {
		var memberID *uuid.UUID
		if member.ID != uuid.Nil {
			memberID = &member.ID
		}
		members = append(members, &event.ThreadMember{ID: memberID, ContactID: member.ContactID, Role: int(member.ThreadRole)})
	}

	events := []ThreadEvent{
		event.NewThreadCreatedBuilder().
			WithDomainID(int32(thread.DomainID)).
			WithCreatedAt(thread.CreatedAt).
			WithID(thread.ID).
			WithMembers(members).
			WithSubject(to.Identity.Name).
			WithKind(thread.Kind.String()).
			WithRecipient(&event.Recipient{
				ID:   from.ID,
				Name: from.Identity.Name,
			}).
			Build(),
		event.NewThreadCreatedBuilder().
			WithDomainID(int32(thread.DomainID)).
			WithCreatedAt(thread.CreatedAt).
			WithID(thread.ID).
			WithMembers(members).
			WithSubject(from.Identity.Name).
			WithKind(thread.Kind.String()).
			WithRecipient(&event.Recipient{
				ID:   to.ID,
				Name: to.Identity.Name,
			}).
			Build(),
	}

	return events, nil
}

func (t *ThreadManagementService) publishThreadCreatedEvents(ctx context.Context, uow store.UnitOfWork, events ...ThreadEvent) error {
	for _, e := range events {
		if err := uow.Outbox().Publish(ctx, e.Topic(), e); err != nil {
			return err
		}
	}
	return nil
}
