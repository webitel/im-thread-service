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
	"google.golang.org/grpc/codes"
)

var (
	notThreadMemberError           = errors.New("member is not part of thread")
	memberAddedSystemMessageType   = "member_added"
	memberRemovedSystemMessageType = "member_removed"
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
		t.logger.Error("searching threads", "operation", "service.thread_manager.search", "err", err)
		return nil, err
	}

	return threads, nil
}

func (t *ThreadManagementService) SearchLeft(ctx context.Context, req *dto.SearchLeftRequest) ([]*model.Thread, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}
	if req.MemberID == uuid.Nil {
		return nil, errors.InvalidArgument("member_id is required")
	}

	query := queryobject.NewSearchLeftQueryObject(req.MemberID).
		WithFields(req.Fields).
		WithDomainIDFilter(req.DomainID).
		WithKindFilter(req.Kinds...).
		WithLimit(req.Size).
		WithSort(req.Sort).
		WithOffset(req.Page)

	return t.uow.ThreadStore().SearchLeft(ctx, query)
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
		if actor.ContactID == initiatorContactID {
			initiatorActor = actor
		}
		if actor.ContactID == targetContactID {
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
		return uuid.Nil, errors.New("failed to find actors", errors.WithCause(err))
	}
	if target != nil {
		return uuid.Nil, errors.New("target member is already part of the thread", errors.WithCode(codes.AlreadyExists), errors.WithID("service.thread_manager.thread_manager"))
	}

	var (
		titleForTarget = "New thread"
		invitedBy      *uuid.UUID
		domainID       int
	)
	if initiator != nil {
		domainID = initiator.DomainID
	}
	if req.InitiatorContactID != uuid.Nil {
		err = t.verifyAddMember(ctx, initiator, req.NewMemberContactID, req.NewMemberRole)
		if err != nil {
			return uuid.Nil, err
		}
		titleForTarget = initiator.Settings.Title
		invitedBy = &initiator.ID
	}

	rolePermissions, err := getDefaultPermissionsByRole(req.NewMemberRole)
	if err != nil {
		return uuid.Nil, err
	}
	var newMember *model.ThreadDialogExtended
	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		newMember, err = uow.ThreadDialogStore().Create(ctx, &model.ThreadDialogExtended{
			BaseModel: shared.BaseModel{
				DomainID:  domainID,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			ThreadID:    req.ThreadID,
			ContactID:   req.NewMemberContactID,
			ThreadRole:  req.NewMemberRole,
			InvitedBy:   invitedBy,
			Permissions: *rolePermissions,
			Settings: model.BaseThreadSetting{
				Title: titleForTarget,
			},
		})
		if err != nil {
			return err
		}
		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs: []uuid.UUID{req.ThreadID},
		})
		if err != nil {
			return errors.Internal("search of members failed", errors.WithCause(err))
		}
		err = t.sendAddMemberSystemMessage(ctx, uow, &addMemberEventArgs{
			initiator: initiator,
			newMember: newMember,
			receivers: eventReceivers,
			threadID:  req.ThreadID,
			domainID:  newMember.DomainID,
		})
		if err != nil {
			return errors.Internal("failed to send system message", errors.WithCause(err))
		}

		return nil

	})
	if err != nil {
		return uuid.Nil, err
	}
	return newMember.ID, nil
}

func (t *ThreadManagementService) verifyAddMember(ctx context.Context, initiator *model.ThreadDialogExtended, targetContactID uuid.UUID, role model.ThreadRole) error {
	if initiator == nil {
		return errors.NotFound("thread not found or initiator does not have permission", errors.WithCause(notThreadMemberError))
	}
	if initiator.ContactID == targetContactID {
		return errors.InvalidArgument("cannot add self as member")
	}
	err := t.verifyAddMemberInitiatorPermissions(initiator.ThreadRole, role, &initiator.Permissions)
	if err != nil {
		return err
	}
	err = t.verifyAddMemberTargetPrivacy(ctx, initiator.ContactID, targetContactID)
	if err != nil {
		return err
	}

	return nil
}

func (t *ThreadManagementService) verifyAddMemberInitiatorPermissions(initiatorRole model.ThreadRole, targetRole model.ThreadRole, initiatorPermissions *model.ThreadPermissions) error {
	if initiatorPermissions == nil {
		return errors.InvalidArgument("permissions cannot be nil")
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

type addMemberEventArgs struct {
	initiator *model.ThreadDialogExtended
	newMember *model.ThreadDialogExtended
	receivers []*model.ThreadDialog
	threadID  uuid.UUID
	domainID  int
}

type removeMemberEventArgs struct {
	initiator *model.ThreadDialogExtended
	member    *model.ThreadDialogExtended
	receivers []*model.ThreadDialog
	reason    *string
	domainID  int
}

func (t *ThreadManagementService) sendAddMemberSystemMessage(ctx context.Context, uow store.UnitOfWork, args *addMemberEventArgs) error {
	if args == nil {
		return errors.New("add member event args cannot be nil")
	}
	if args.newMember == nil {
		return errors.New("target member cannot be nil")
	}
	if args.receivers == nil {
		return errors.New("message recipients cannot be nil")
	}
	if args.domainID <= 0 {
		return errors.InvalidArgument("domain id must be greater than zero", errors.WithID("service.thread_manager.send_add_member_system_message"))
	}
	var (
		newMember     = args.newMember
		systemMessage = &model.MessageSystem{
			Type: memberAddedSystemMessageType,
			Metadata: map[string]any{
				"new_member_id":         newMember.ID,
				"new_member_contact_id": newMember.ContactID,
				"new_member_role":       newMember.ThreadRole,
				"thread_id":             args.threadID,
			},
		}
		message = &model.Message{
			IdempotencyKey: uuid.NewString(),
			ThreadID:       args.threadID,
			DomainID:       int32(args.domainID),
			To:             args.receivers,
			Type:           model.MessageTypeSystem,
			System:         systemMessage,
			Metadata:       model.BuildMetadata(""),
			SendTo: shared.Peer{
				ID:   newMember.ContactID,
				Type: shared.PeerContact,
			},
		}
		initiator = args.initiator
	)
	if initiator != nil {
		message.Member = &model.ThreadDialog{
			BaseModel: shared.BaseModel{
				ID: initiator.ID,
			},
			ContactID:  initiator.ContactID,
			ThreadID:   initiator.ThreadID,
			ThreadRole: initiator.ThreadRole,
		}
	}

	return t.sendThreadSystemMessage(ctx, uow, message)
}

func (s *ThreadManagementService) sendThreadSystemMessage(ctx context.Context, uow store.UnitOfWork, msg *model.Message) error {
	if msg.ThreadID == uuid.Nil {
		return errors.New("thread id cannot be nil")
	}
	if msg.DomainID <= 0 {
		return errors.New("domain id must be greater than zero")
	}
	if msg.To == nil {
		return errors.New("message recipients cannot be nil")
	}
	savedMsg, err := uow.Messages().SaveSystemMessage(ctx, msg)
	if err != nil {
		return err
	}
	savedMsg.WithCreatedEvent(uuid.NewString(), &msg.From)
	events := savedMsg.Events()

	for _, e := range events {
		if err = uow.Outbox().Publish(ctx, buildMessageCreatedTopic(e.RecipientID(), e.Version()), e); err != nil {
			return err
		}
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
		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs:      []uuid.UUID{target.ThreadID},
			IncludeDeleted: false,
		})
		if err != nil {
			return err
		}
		err = uow.ThreadDialogStore().Delete(ctx, target.ID, req.Reason)
		if err != nil {
			return err
		}
		domainID := target.DomainID
		if domainID <= 0 && initiator != nil {
			domainID = initiator.DomainID
		}
		err = t.sendRemoveMemberSystemMessage(ctx, uow, &removeMemberEventArgs{
			initiator: initiator,
			member:    target,
			receivers: eventReceivers,
			reason:    req.Reason,
			domainID:  domainID,
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (t *ThreadManagementService) sendRemoveMemberSystemMessage(ctx context.Context, uow store.UnitOfWork, args *removeMemberEventArgs) error {
	if args == nil {
		return errors.New("remove member event args cannot be nil")
	}
	if args.member == nil {
		return errors.New("removed member cannot be nil")
	}
	if args.receivers == nil {
		return errors.New("message recipients cannot be nil")
	}
	if args.domainID <= 0 {
		return errors.InvalidArgument("domain id must be greater than zero", errors.WithID("service.thread_manager.send_remove_member_system_message"))
	}

	removedMember := args.member
	metadata := map[string]any{
		"removed_member_id":         removedMember.ID,
		"removed_member_contact_id": removedMember.ContactID,
		"thread_id":                 removedMember.ThreadID,
	}
	if args.reason != nil {
		metadata["reason"] = *args.reason
	}
	message := &model.Message{
		IdempotencyKey: uuid.NewString(),
		ThreadID:       removedMember.ThreadID,
		DomainID:       int32(args.domainID),
		To:             args.receivers,
		Type:           model.MessageTypeSystem,
		Metadata:       model.BuildMetadata(""),
		System: &model.MessageSystem{
			Type:     memberRemovedSystemMessageType,
			Metadata: metadata,
		},
		SendTo: shared.Peer{
			ID:   removedMember.ContactID,
			Type: shared.PeerContact,
		},
	}
	if args.initiator != nil {
		message.Member = &model.ThreadDialog{
			BaseModel: shared.BaseModel{
				ID: args.initiator.ID,
			},
			ContactID:  args.initiator.ContactID,
			ThreadID:   args.initiator.ThreadID,
			ThreadRole: args.initiator.ThreadRole,
		}
	}

	return t.sendThreadSystemMessage(ctx, uow, message)
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
		return errors.InvalidArgument("permissions cannot be nil", errors.WithID("service.thread_manager.verify_remove_member_initiator_permissions"))
	}
	if !initiatorPermissions.CanRemoveMembers {
		return errors.Forbidden("initiator does not have permission to remove members", errors.WithID("service.thread_manager.verify_remove_member_initiator_permissions"))
	}
	if initiatorRole <= targetRole {
		return errors.Forbidden("initiator does not have permission to remove members", errors.WithID("service.thread_manager.verify_remove_member_initiator_permissions"))
	}

	return nil
}

func (t *ThreadManagementService) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_manager.ensure_direct_thread"))
	}

	log := t.logger.With("operation", "ensure_direct_thread")

	thread, err := t.searchThread(ctx, req.From, req.To)
	if err != nil {
		log.ErrorContext(ctx, "searching thread", "err", err)
		return nil, err
	}

	if thread == nil {
		if thread, err = t.orchestrateDirectThreadCreation(ctx, req); err != nil {
			log.ErrorContext(ctx, "creating thread", "err", err)
			return nil, err
		}
	}

	return thread, nil
}

func (t *ThreadManagementService) searchThread(ctx context.Context, from, to *shared.Peer) (*model.Thread, error) {
	if from == nil {
		return nil, errors.InvalidArgument("from peer cannot be nil", errors.WithID("service.thread_manager.search_direct_thread"))
	}

	if to == nil {
		return nil, errors.InvalidArgument("recipient peer cannot be nil", errors.WithID("service.thread_manager.search_direct_thread"))
	}

	resolveThreadQuery := model.ResolveThreadQuery{From: *from, To: *to}

	thread, err := t.uow.ThreadStore().ResolveThread(ctx, resolveThreadQuery)
	if err != nil {
		return nil, err
	}

	return thread, nil
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
		ContactID:  tde.ContactID,
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
			ContactID:   from.ID,
			ThreadRole:  initiatorRole,
			Permissions: *initiatorPermissions,
			Settings: model.BaseThreadSetting{
				Title: to.Identity.Name,
			},
		}
		targerPermissions = &model.ThreadDialogExtended{
			BaseModel:   baseModel,
			ThreadID:    threadID,
			ContactID:   to.ID,
			ThreadRole:  peerRole,
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
