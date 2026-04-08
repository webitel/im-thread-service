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
	"github.com/webitel/im-thread-service/internal/utils"
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
		WithDomainIDFilter(searchRequest.DomainIds...).
		WithKindFilter(searchRequest.Kinds...).
		WithMemberIDFilter(searchRequest.MemberIds...).
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

func (t *ThreadManagementService) AddMember(ctx context.Context, req *dto.AddMemberRequest) error {
	if req == nil {
		return errors.New("add member request cannot be nil")
	}

	initiator, target, err := t.findActionActorsPersistence(ctx, req.ThreadID, req.InitiatorMemberID, req.NewMemberID)
	if err != nil {
		return err
	}
	if initiator == nil {
		return notThreadMemberError
	}
	if target != nil {
		return errors.New("target member is already part of the thread")
	}

	err = t.verifyAddMemberInitiatorPermissions(initiator.ThreadRole, req.NewMemberRole, &initiator.Permissions)
	if err != nil {
		return err
	}
	err = t.verifyAddMemberTargetPrivacy(ctx, req.InitiatorMemberID, req.NewMemberID)
	if err != nil {
		return err
	}

	rolePermissions, err := getDefaultPermissionsByRole(req.NewMemberRole)
	if err != nil {
		return err
	}
	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		createdThreadDialog, err := t.uow.ThreadDialogStore().Create(ctx, &model.ThreadDialogExtended{
			BaseModel: shared.BaseModel{
				DomainID:  initiator.DomainID,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			ThreadID:    req.ThreadID,
			MemberID:    req.NewMemberID,
			ThreadRole:  req.NewMemberRole,
			Permissions: *rolePermissions,
			Settings: model.BaseThreadSetting{
				Title: initiator.Settings.Title,
			},
		})
		if err != nil {
			return err
		}

		// TODO: optimize publishing, so we can reduce round trips to the store
		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs: []uuid.UUID{req.ThreadID},
		})
		if err != nil {
			return err
		}
		for _, receiver := range eventReceivers {
			event := &event.MemberAddedV1{
				ThreadID:          req.ThreadID,
				MemberID:          req.NewMemberID,
				NewThreadDialogID: createdThreadDialog.ID,
				InitiatorMemberID: req.InitiatorMemberID,
				Recipient:         receiver.MemberID,
			}
			uow.Outbox().Publish(ctx, event.Topic(), event)
		}
		return nil

	})
	if err != nil {
		return err
	}
	return nil
}

func (t *ThreadManagementService) verifyAddMemberInitiatorPermissions(initiatorRole model.ThreadRole, targetRole model.ThreadRole, initiatorPermissions *model.ThreadPermissions) error {
	if initiatorPermissions == nil {
		return errors.New("permissions cannot be nil")
	}
	if !initiatorPermissions.CanAddMembers {
		return errors.New("initiator does not have permission to invite members")
	}
	if initiatorRole <= targetRole {
		return errors.New("initiator does not have permission to invite a member with same or higher role")
	}

	return nil
}
func (t *ThreadManagementService) verifyAddMemberTargetPrivacy(ctx context.Context, initiatorID, targetID uuid.UUID) error {
	err := t.privacyChecker.CanInvite(ctx, initiatorID, targetID)
	if err != nil {
		return err
	}
	return nil
}

func (t *ThreadManagementService) findActionActorsPersistence(ctx context.Context, threadID uuid.UUID, initiatorID uuid.UUID, targetID uuid.UUID) (initiator *model.ThreadDialogExtended, target *model.ThreadDialogExtended, err error) {
	threadDialogs, err := t.uow.ThreadDialogStore().GetFullView(ctx, &model.ThreadDialogStoreFilter{
		ThreadIDs: []uuid.UUID{threadID},
		MemberIDs: []uuid.UUID{initiatorID, targetID},
	})
	if err != nil {
		return nil, nil, err
	}
	if len(threadDialogs) == 0 {
		return nil, nil, nil
	}
	for _, dialog := range threadDialogs {
		if dialog.MemberID == initiatorID {
			initiator = dialog
		}
		if dialog.MemberID == targetID {
			target = dialog
		}
	}
	return initiator, target, nil
}

func (t *ThreadManagementService) RemoveMember(ctx context.Context, req *dto.RemoveMemberRequest) error {
	if req == nil {
		return errors.New("remove member request cannot be nil")
	}
	initiator, target, err := t.findActionActorsPersistence(ctx, req.ThreadID, req.InitiatorMemberID, req.TargetMemberID)
	if err != nil {
		return err
	}
	if initiator == nil {
		return notThreadMemberError
	}
	if target == nil {
		return errors.New("target member is not part of the thread")
	}

	err = t.verifyRemoveMemberInitiatorPermissions(initiator.ThreadRole, target.ThreadRole, &initiator.Permissions)
	if err != nil {
		return err
	}

	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		err = uow.ThreadDialogStore().Delete(ctx, req.ThreadID, req.TargetMemberID)
		if err != nil {
			return err
		}
		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs: []uuid.UUID{req.ThreadID},
		})
		if err != nil {
			return err
		}
		for _, receiver := range eventReceivers {
			event := &event.MemberRemovedV1{
				ThreadID:              req.ThreadID,
				RemovedMemberID:       req.TargetMemberID,
				RemovedThreadDialogID: target.ID,
				InitiatorMemberID:     req.InitiatorMemberID,
				Recipient:             receiver.ID,
			}

			err = uow.Outbox().Publish(ctx, event.Topic(), event)
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

func (t *ThreadManagementService) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*dto.EnsureDirectThreadResponse, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_manager.ensure_direct_thread"))
	}

	directThread, err := t.searchDirectThread(ctx, req.From, req.To)
	if err != nil {
		return nil, err
	}

	if directThread != nil {
		members := utils.Map(directThread.Members, func(m *model.ThreadDialog) uuid.UUID {
			return m.MemberID
		})
		return dto.NewEnsureDirectThreadResponse(directThread.ID, int32(directThread.DomainID), members), nil
	}

	directThread, err = t.orchestrateDirectThreadCreation(ctx, req)
	if err != nil {
		return nil, err
	}

	return dto.NewEnsureDirectThreadResponse(directThread.ID, int32(directThread.DomainID), uuid.UUIDs{req.From.ID, req.To.ID}), nil
}

func (t *ThreadManagementService) searchDirectThread(ctx context.Context, from, to *shared.Peer) (*model.Thread, error) {
	if from == nil || to == nil {
		return nil, errors.New("from and to peers cannot be nil")
	}
	var (
		thread *model.Thread
		err    error
	)

	switch to.Type {
	case shared.PeerContact:
		thread, err = t.uow.ThreadStore().ResolveDirect(ctx, from.ID, to.ID)
		if err != nil {
			return nil, err
		}
	case shared.PeerThread:
		threadID := to.ID
		threads, err := t.uow.ThreadStore().Search(
			ctx,
			queryobject.NewThreadQueryObject().
				WithIDFilter(threadID).
				WithMemberIDFilter(from.ID).
				WithKindFilter(model.ThreadDirect).
				WithFields([]string{"id", "domain_id", "created_at", "updated_at", "kind", "owner", "subject", "description", "members"}).
				WithLimit(1),
		)
		if err != nil {
			return nil, err
		}
		if len(threads) == 0 {
			return nil, nil
		}
		thread = threads[0]
	default:
		return nil, errors.New("invalid peer type for direct")
	}

	return thread, nil

}

func (t *ThreadManagementService) orchestrateDirectThreadCreation(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	var createdThread *model.Thread
	err := t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		savedThread, err := t.createDirectThread(ctx, uow, req.DomainID, req.From, req.To)
		if err != nil {
			return err
		}
		createdThread = savedThread

		if _, err = t.initializeDirectThreadDialogs(ctx, uow, createdThread.ID, req.DomainID, req.From, req.To); err != nil {
			return err
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
		return nil, errors.New("thread, from and to cannot be nil")
	}
	var (
		events     []ThreadEvent
		membersIDs = []uuid.UUID{from.ID, to.ID}
	)
	events = append(events,
		event.NewThreadCreatedBuilder().
			WithDomainID(int32(thread.DomainID)).
			WithCreatedAt(thread.CreatedAt).
			WithID(thread.ID).
			WithMembers(membersIDs).
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
			WithMembers(membersIDs).
			WithSubject(from.Identity.Name).
			WithKind(thread.Kind.String()).
			WithRecipient(&event.Recipient{
				ID:   to.ID,
				Name: to.Identity.Name,
			}).
			Build(),
	)

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
