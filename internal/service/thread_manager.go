package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

var (
	errNotThreadMemberError        = errors.New("member is not part of thread")
	memberAddedSystemMessageType   = "member_added"
	memberRemovedSystemMessageType = "member_removed"
)

const (
	memberTransferedSystemMessageType = "transferred"
	memberTransferedLeaveReason       = "transferred"
)

type (
	ThreadManagementService struct {
		uow            store.UnitOfWork
		logger         *slog.Logger
		privacyChecker ThreadPrivacyChecker
		contactInfo    ContactInfoProvider
	}

	ThreadPrivacyChecker interface {
		CanInvite(ctx context.Context, initiatorID, targetID uuid.UUID) error
		CanSend(ctx context.Context, senderID, recipientID uuid.UUID) error
	}

	ContactInfoProvider interface {
		IsBot(ctx context.Context, contactID uuid.UUID, domainID int) (bool, error)
	}

	ThreadEvent interface {
		event.Outboxer

		Topic() string
		MustBeThreadEvent()
	}
)

// NewThreadService returns a new thread manager, given a unit of work.
func NewThreadService(logger *slog.Logger, uow store.UnitOfWork, privacyChecker ThreadPrivacyChecker, contactInfo ContactInfoProvider) *ThreadManagementService {
	log := logger.With(slog.String("component", "thread"))

	return &ThreadManagementService{
		uow:            uow,
		logger:         log,
		privacyChecker: privacyChecker,
		contactInfo:    contactInfo,
	}
}

func (t *ThreadManagementService) Get(ctx context.Context, req *dto.ThreadGetRequest) (*model.Thread, error) {
	query := queryobject.NewThreadQueryObject().
		WithIDFilter(req.ID).
		WithDomainIDFilter(req.DomainID).
		WithFields(req.Fields)

	thread, err := t.uow.ThreadStore().Get(ctx, query)
	if err != nil {
		t.logger.Error("getting thread", "operation", "service.thread_manager.get", "id", req.ID, "err", err)

		return nil, err
	}

	return thread, nil
}

func (t *ThreadManagementService) Search(ctx context.Context, searchRequest *dto.ThreadSearchRequest) ([]*model.Thread, error) {
	if searchRequest == nil {
		return nil, errors.New("search request cannot be nil")
	}

	query := queryobject.NewThreadQueryObject().
		WithSubject().
		WithFields(searchRequest.Fields).
		WithIDFilter(searchRequest.IDs...).
		WithDomainIDFilter(searchRequest.DomainIDs...).
		WithKindFilter(searchRequest.Kinds...).
		WithOwnerFilter(searchRequest.Owners...).
		WithSubjectFilter(searchRequest.Q).
		WithLimit(searchRequest.Size).
		WithSort(searchRequest.Sort).
		WithoutDeletedAtFilter().
		WithOffset(searchRequest.Page)

	if len(searchRequest.ContactIDs) > 0 {
		query = query.WithSharedMembersFilter(searchRequest.SelfID, searchRequest.ContactIDs...)
	} else {
		query = query.WithContactIDFilter(searchRequest.SelfID)
	}

	threads, err := t.uow.ThreadStore().Search(ctx, query)
	if err != nil {
		t.logger.Error("searching threads", "operation", "service.thread_manager.search", "err", err)

		return nil, err
	}

	return threads, nil
}

func (t *ThreadManagementService) SearchLeft(ctx context.Context, req *dto.SearchLeftRequest) ([]*model.Thread, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_manager."))
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

func (t *ThreadManagementService) findAddMemberActors(ctx context.Context, threadID, initiatorContactID, targetContactID uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error) {
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
		domainID       = req.DomainID
	)
	if initiator != nil {
		domainID = initiator.DomainID
	}

	if req.InitiatorContactID != uuid.Nil {
		err = t.verifyAddMember(ctx, initiator, req.NewMemberContactID, req.NewMemberRole)
		if err != nil {
			return uuid.Nil, err
		}

		if initiator != nil {
			titleForTarget = initiator.Settings.Title
			invitedBy = &initiator.ID
		}
	}

	if t.contactInfo != nil && !req.IsBot {
		t.logger.InfoContext(ctx, "checking is_bot for contact", "contact_id", req.NewMemberContactID, "domain_id", domainID)

		isBot, err := t.contactInfo.IsBot(ctx, req.NewMemberContactID, domainID)
		if err != nil {
			t.logger.WarnContext(ctx, "failed to check is_bot for contact, assuming false", "contact_id", req.NewMemberContactID, "err", err)
		} else {
			t.logger.InfoContext(ctx, "is_bot check result", "contact_id", req.NewMemberContactID, "is_bot", isBot)
			req.IsBot = isBot
		}
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
			IsBot:       req.IsBot,
			AutoLeave:   req.IsBot && resolveAutoLeave(req.AutoLeave),
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

		if req.IsBot {
			t.logger.DebugContext(ctx, "bot member added: pushing bot control stack",
				"thread_id", req.ThreadID,
				"member_id", newMember.ID,
				"reason", model.BotControlReasonTransfer,
			)

			pushResult, pushErr := uow.BotControl().Push(ctx, model.BotControlTransition{
				ThreadID:    req.ThreadID,
				NewMemberID: newMember.ID,
				Reason:      model.BotControlReasonTransfer,
				TriggeredBy: invitedBy,
			})
			if pushErr != nil {
				t.logger.ErrorContext(ctx, "failed to push bot control stack",
					"thread_id", req.ThreadID,
					"member_id", newMember.ID,
					"err", pushErr,
				)
				return errors.Internal("failed to push bot control", errors.WithCause(pushErr))
			}

			t.logger.DebugContext(ctx, "bot control stack pushed",
				"thread_id", req.ThreadID,
				"member_id", newMember.ID,
				"control_epoch", pushResult.ControlEpoch,
				"prev_member_id", pushResult.Prev,
			)

			prev := pushResult.Prev
			newPos := positionAfterPush(prev)

			if prev != nil && prev.MemberID != nil {
				if err = t.publishBotControlReleased(ctx, uow, newMember.ThreadID, *prev.MemberID, prev.Position, domainID, &newMember.ID, model.BotControlReasonTransfer); err != nil {
					return err
				}
			}

			if err = t.publishBotControlGranted(ctx, uow, newMember, prev, newPos, model.BotControlReasonTransfer, false, pushResult.ControlEpoch); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}

	return newMember.ID, nil
}

func (t *ThreadManagementService) Transfer(ctx context.Context, req *dto.TransferThreadRequest) (uuid.UUID, error) {
	if req == nil {
		return uuid.Nil, errors.New("transfer request cannot be nil")
	}

	if req.InitiatorContactID == uuid.Nil {
		return uuid.Nil, errors.InvalidArgument("initiator contact id cannot be nil")
	}

	initiator, target, err := t.findAddMemberActors(ctx, req.ThreadID, req.InitiatorContactID, req.NewMemberContactID)
	if err != nil {
		return uuid.Nil, errors.New("failed to find actors", errors.WithCause(err))
	}

	if initiator == nil {
		return uuid.Nil, errors.NotFound("thread not found or initiator does not have permission", errors.WithCause(errNotThreadMemberError))
	}

	if target != nil {
		return uuid.Nil, errors.New("target member is already part of the thread", errors.WithCode(codes.AlreadyExists), errors.WithID("service.thread_manager.transfer"))
	}

	rolePermissions, err := getDefaultPermissionsByRole(req.NewMemberRole)
	if err != nil {
		return uuid.Nil, err
	}

	if initiator.ThreadRole == req.NewMemberRole { // inherit permissions if roles are the same
		rolePermissions = &initiator.Permissions
	}

	err = t.verifyAddMember(ctx, initiator, req.NewMemberContactID, req.NewMemberRole)
	if err != nil {
		return uuid.Nil, err
	}

	var newMemberID uuid.UUID

	err = t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		now := time.Now().UTC()

		newMember, err := uow.ThreadDialogStore().Create(ctx, &model.ThreadDialogExtended{
			BaseModel: shared.BaseModel{
				DomainID:  initiator.DomainID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			ThreadID:    req.ThreadID,
			ContactID:   req.NewMemberContactID,
			ThreadRole:  req.NewMemberRole,
			InvitedBy:   &initiator.ID,
			Permissions: *rolePermissions,
			IsBot:       req.TargetIsBot,
			AutoLeave:   req.TargetIsBot && resolveAutoLeave(req.AutoLeave),
			Settings: model.BaseThreadSetting{
				Title: initiator.Settings.Title,
			},
		})
		if err != nil {
			return err
		}

		transferReason := memberTransferedLeaveReason
		if err = uow.ThreadDialogStore().Delete(ctx, initiator.ID, &transferReason); err != nil {
			return err
		}

		eventReceivers, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs:      []uuid.UUID{req.ThreadID},
			IncludeDeleted: false,
		})
		if err != nil {
			return errors.Internal("search of members failed", errors.WithCause(err))
		}

		err = t.sendTransferSystemMessage(ctx, uow, &transferMemberEventArgs{
			initiator: initiator,
			newMember: newMember,
			receivers: eventReceivers,
			threadID:  req.ThreadID,
			domainID:  newMember.DomainID,
		})
		if err != nil {
			return errors.Internal("failed to send system message", errors.WithCause(err))
		}

		if req.TargetIsBot {
			t.logger.DebugContext(ctx, "transfer: target is bot, pushing bot control stack",
				"thread_id", req.ThreadID,
				"member_id", newMember.ID,
			)

			pushResult, pushErr := uow.BotControl().Push(ctx, model.BotControlTransition{
				ThreadID:    req.ThreadID,
				NewMemberID: newMember.ID,
				Reason:      model.BotControlReasonTransfer,
				TriggeredBy: &initiator.ID,
			})
			if pushErr != nil {
				t.logger.ErrorContext(ctx, "transfer: failed to push bot control stack",
					"thread_id", req.ThreadID,
					"member_id", newMember.ID,
					"err", pushErr,
				)
				return errors.Internal("failed to push bot control", errors.WithCause(pushErr))
			}

			t.logger.DebugContext(ctx, "transfer: bot control stack pushed",
				"thread_id", req.ThreadID,
				"member_id", newMember.ID,
				"control_epoch", pushResult.ControlEpoch,
			)

			prev := pushResult.Prev
			newPos := positionAfterPush(prev)

			if prev != nil && prev.MemberID != nil {
				if err = t.publishBotControlReleased(ctx, uow, newMember.ThreadID, *prev.MemberID, prev.Position, initiator.DomainID, &newMember.ID, model.BotControlReasonTransfer); err != nil {
					return err
				}
			}

			if err = t.publishBotControlGranted(ctx, uow, newMember, prev, newPos, model.BotControlReasonTransfer, false, pushResult.ControlEpoch); err != nil {
				return err
			}
		}

		newMemberID = newMember.ID

		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}

	return newMemberID, nil
}

func (t *ThreadManagementService) verifyAddMember(ctx context.Context, initiator *model.ThreadDialogExtended, targetContactID uuid.UUID, role model.ThreadRole) error {
	if initiator == nil {
		return errors.NotFound("thread not found or initiator does not have permission", errors.WithCause(errNotThreadMemberError))
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

func (t *ThreadManagementService) verifyAddMemberInitiatorPermissions(initiatorRole, targetRole model.ThreadRole, initiatorPermissions *model.ThreadPermissions) error {
	if initiatorPermissions == nil {
		return errors.InvalidArgument("permissions cannot be nil")
	}

	if !initiatorPermissions.CanAddMembers {
		return errors.Forbidden("initiator does not have permission to invite members")
	}

	if initiatorRole < targetRole {
		return errors.Forbidden("initiator does not have permission to invite a member with higher role")
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

type transferMemberEventArgs struct {
	initiator *model.ThreadDialogExtended
	newMember *model.ThreadDialogExtended
	receivers []*model.ThreadDialog
	threadID  uuid.UUID
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
		body          = "Member added"
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
			Body:           body,
			Metadata:       model.BuildMetadata(body),
			SendTo: shared.Peer{
				ID:   newMember.ContactID,
				Type: shared.PeerContact,
			},
		}
		initiator = args.initiator
	)
	if initiator != nil {
		message.SenderID = initiator.ContactID
		message.From = shared.Peer{
			ID:   initiator.ContactID,
			Type: shared.PeerContact,
		}
		message.Member = &model.ThreadDialog{
			BaseModel: shared.BaseModel{
				ID: initiator.ID,
			},
			ContactID:  initiator.ContactID,
			ThreadID:   initiator.ThreadID,
			ThreadRole: initiator.ThreadRole,
		}
	}

	savedMsg, err := t.sendThreadSystemMessage(ctx, uow, message)
	if err != nil {
		return err
	}

	joinedEvent := &event.MemberJoined{
		MessageID:  savedMsg.ID,
		ThreadID:   args.threadID,
		DomainID:   int32(args.domainID),
		ContactID:  newMember.ContactID,
		OccurredAt: savedMsg.CreatedAt,
		System:     event.NewSystemPayload(systemMessage.Type, systemMessage.Metadata),
	}

	return t.publishMemberEvent(ctx, uow, joinedEvent)
}

func (t *ThreadManagementService) sendThreadSystemMessage(ctx context.Context, uow store.UnitOfWork, msg *model.Message) (*model.Message, error) {
	if msg.ThreadID == uuid.Nil {
		return nil, errors.New("thread id cannot be nil")
	}

	if msg.DomainID <= 0 {
		return nil, errors.New("domain id must be greater than zero")
	}

	if msg.To == nil {
		return nil, errors.New("message recipients cannot be nil")
	}

	savedMsg, err := uow.Messages().SaveSystemMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	savedMsg.To = msg.To
	savedMsg.From = msg.From
	savedMsg.WithCreatedEvent(ctx, uuid.NewString())
	events := savedMsg.Events()

	for _, e := range events {
		if err = uow.Outbox().Publish(ctx, buildMessageCreatedTopic(e.RecipientID(), e.Version()), e); err != nil {
			return nil, err
		}
	}

	return savedMsg, nil
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

		domainID := target.DomainID
		if domainID <= 0 && initiator != nil {
			domainID = initiator.DomainID
		}

		if target.IsBot {
			var triggeredBy *uuid.UUID
			if initiator != nil {
				triggeredBy = &initiator.ID
			}

			newTop, popErr := uow.BotControl().Pop(ctx, target.ThreadID, target.ID, model.BotControlReasonRemoved, triggeredBy)
			if popErr != nil {
				return errors.Internal("failed to pop bot control", errors.WithCause(popErr))
			}

			var nextMemberID *uuid.UUID
			if newTop != nil {
				nextMemberID = newTop.MemberID
			}

			if err = t.publishBotControlReleased(ctx, uow, target.ThreadID, target.ID, 0, domainID, nextMemberID, model.BotControlReasonRemoved); err != nil {
				return err
			}

			if newTop != nil && newTop.MemberID != nil {
				newTopDialog := botControlStackEntryToDialog(newTop)
				if err = t.publishBotControlGranted(ctx, uow, newTopDialog, &model.BotControlStackEntry{
					MemberID: &target.ID,
					Position: newTop.Position + 1,
				}, newTop.Position, model.BotControlReasonRemoved, true, newTop.ControlEpoch); err != nil {
					return err
				}
			}

			// Pop already soft-deletes auto_leave dialogs; only delete permanent ones manually
			if !target.AutoLeave {
				if err = uow.ThreadDialogStore().Delete(ctx, target.ID, req.Reason); err != nil {
					return err
				}
			}
		} else {
			if err = uow.ThreadDialogStore().Delete(ctx, target.ID, req.Reason); err != nil {
				return err
			}
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

	body := "Member removed"

	message := &model.Message{
		IdempotencyKey: uuid.NewString(),
		ThreadID:       removedMember.ThreadID,
		DomainID:       int32(args.domainID),
		To:             args.receivers,
		Type:           model.MessageTypeSystem,
		Body:           body,
		Metadata:       model.BuildMetadata(body),
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
		message.SenderID = args.initiator.ContactID
		message.From = shared.Peer{
			ID:   args.initiator.ContactID,
			Type: shared.PeerContact,
		}
		message.Member = &model.ThreadDialog{
			BaseModel: shared.BaseModel{
				ID: args.initiator.ID,
			},
			ContactID:  args.initiator.ContactID,
			ThreadID:   args.initiator.ThreadID,
			ThreadRole: args.initiator.ThreadRole,
		}
	}

	savedMsg, err := t.sendThreadSystemMessage(ctx, uow, message)
	if err != nil {
		return err
	}

	leftEvent := &event.MemberLeft{
		MessageID:  savedMsg.ID,
		ThreadID:   removedMember.ThreadID,
		DomainID:   int32(args.domainID),
		ContactID:  removedMember.ContactID,
		OccurredAt: savedMsg.CreatedAt,
		System:     event.NewSystemPayload(memberRemovedSystemMessageType, metadata),
	}

	return t.publishMemberEvent(ctx, uow, leftEvent)
}

func (t *ThreadManagementService) sendTransferSystemMessage(ctx context.Context, uow store.UnitOfWork, args *transferMemberEventArgs) error {
	if args == nil {
		return errors.New("transfer member event args cannot be nil")
	}

	if args.initiator == nil {
		return errors.New("initiator member cannot be nil")
	}

	if args.newMember == nil {
		return errors.New("new member cannot be nil")
	}

	if args.receivers == nil {
		return errors.New("message recipients cannot be nil")
	}

	if args.domainID <= 0 {
		return errors.InvalidArgument("domain id must be greater than zero", errors.WithID("service.thread_manager.send_transfer_system_message"))
	}

	metadata := map[string]any{
		"new_member_id":                 args.newMember.ID,
		"new_member_contact_id":         args.newMember.ContactID,
		"new_member_role":               args.newMember.ThreadRole,
		"transferred_member_id":         args.initiator.ID,
		"transferred_member_contact_id": args.initiator.ContactID,
		"thread_id":                     args.threadID,
	}

	message := &model.Message{
		IdempotencyKey: uuid.NewString(),
		ThreadID:       args.threadID,
		DomainID:       int32(args.domainID),
		To:             args.receivers,
		Type:           model.MessageTypeSystem,
		System: &model.MessageSystem{
			Type:     memberTransferedSystemMessageType,
			Metadata: metadata,
		},
		Metadata: model.BuildMetadata(""),
		SendTo: shared.Peer{
			ID:   args.newMember.ContactID,
			Type: shared.PeerContact,
		},
		Member: &model.ThreadDialog{
			BaseModel:  shared.BaseModel{ID: args.initiator.ID},
			ContactID:  args.initiator.ContactID,
			ThreadID:   args.initiator.ThreadID,
			ThreadRole: args.initiator.ThreadRole,
		},
	}

	savedMsg, err := t.sendThreadSystemMessage(ctx, uow, message)
	if err != nil {
		return err
	}

	leftEvent := &event.MemberLeft{
		MessageID:  savedMsg.ID,
		ThreadID:   args.threadID,
		DomainID:   int32(args.domainID),
		ContactID:  args.initiator.ContactID,
		OccurredAt: savedMsg.CreatedAt,
		System:     event.NewSystemPayload(memberTransferedSystemMessageType, metadata),
	}
	if err = t.publishMemberEvent(ctx, uow, leftEvent); err != nil {
		return err
	}

	joinedEvent := &event.MemberJoined{
		MessageID:  savedMsg.ID,
		ThreadID:   args.threadID,
		DomainID:   int32(args.domainID),
		ContactID:  args.newMember.ContactID,
		OccurredAt: savedMsg.CreatedAt,
		System:     event.NewSystemPayload(memberTransferedSystemMessageType, metadata),
	}

	return t.publishMemberEvent(ctx, uow, joinedEvent)
}

func (t *ThreadManagementService) verifyRemoveMember(initiator, target *model.ThreadDialogExtended) error {
	if initiator == nil {
		return errors.NotFound("thread not found or initiator does not have permission", errors.WithCause(errNotThreadMemberError))
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

func (t *ThreadManagementService) verifyRemoveMemberInitiatorPermissions(initiatorRole, targetRole model.ThreadRole, initiatorPermissions *model.ThreadPermissions) error {
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

// CompleteBotControl is called by flow_manager when a bot schema finishes execution.
// It pops the bot from the control stack and returns control to the previous controller.
// Unlike RemoveMember, this is a voluntary release — reason logged as "completed".
func (t *ThreadManagementService) CompleteBotControl(ctx context.Context, req *dto.CompleteBotControlRequest) error {
	if req == nil {
		return errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_manager.complete_bot_control"))
	}

	if req.ThreadID == uuid.Nil {
		return errors.InvalidArgument("thread_id is required", errors.WithID("service.thread_manager.complete_bot_control"))
	}

	if req.MemberID == uuid.Nil {
		return errors.InvalidArgument("member_id is required", errors.WithID("service.thread_manager.complete_bot_control"))
	}

	return t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		stack, err := uow.BotControl().GetStack(ctx, req.ThreadID)
		if err != nil {
			return err
		}

		// Only the active controller (top of stack) may complete bot control.
		if len(stack) == 0 || stack[len(stack)-1].MemberID == nil || *stack[len(stack)-1].MemberID != req.MemberID {
			return errors.InvalidArgument("member is not the active bot controller",
				errors.WithID("service.thread_manager.complete_bot_control"))
		}

		// Reject stale requests: epoch must match the value delivered in bot.control.granted.v1.
		currentEpoch, err := uow.BotControl().GetControlEpoch(ctx, req.ThreadID)
		if err != nil {
			return err
		}

		if req.ControlEpoch != currentEpoch {
			return errors.InvalidArgument("control_epoch mismatch — request is stale or already processed",
				errors.WithID("service.thread_manager.complete_bot_control"))
		}

		completedPosition := stack[len(stack)-1].Position

		newTop, err := uow.BotControl().Pop(ctx, req.ThreadID, req.MemberID, model.BotControlReasonCompleted, nil)
		if err != nil {
			return err
		}

		var nextMemberID *uuid.UUID
		if newTop != nil {
			nextMemberID = newTop.MemberID
		}

		if err = t.publishBotControlReleased(ctx, uow, req.ThreadID, req.MemberID, completedPosition, req.DomainID, nextMemberID, model.BotControlReasonCompleted); err != nil {
			return err
		}

		if newTop == nil || newTop.MemberID == nil {
			return nil
		}

		return t.publishBotControlGranted(ctx, uow, botControlStackEntryToDialog(newTop), &model.BotControlStackEntry{
			MemberID: &req.MemberID,
			Position: completedPosition,
		}, newTop.Position, model.BotControlReasonCompleted, true, newTop.ControlEpoch)
	})
}

func (t *ThreadManagementService) EnsureDirectThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) (*model.Thread, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("service.thread_manager.ensure_direct_thread"))
	}

	log := t.logger.With("operation", "ensure_direct_thread")

	searchThreadQuery := model.ResolveThreadQuery{From: *req.From, To: *req.To, SendAs: req.SendAs}

	thread, err := t.searchThread(ctx, searchThreadQuery)
	if err != nil {
		log.ErrorContext(ctx, "searching thread", "err", err)

		return nil, err
	}

	if thread != nil {
		return thread, nil
	}

	if thread, err = t.orchestrateDirectThreadCreation(ctx, req); err != nil {
		log.ErrorContext(ctx, "creating thread", "err", err)

		return nil, err
	}

	return thread, nil
}

func (t *ThreadManagementService) searchThread(ctx context.Context, searchQuery model.ResolveThreadQuery) (*model.Thread, error) {
	thread, err := t.uow.ThreadStore().ResolveThread(ctx, searchQuery)
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

		var toIsBot bool
		if req.ToIsBot != nil {
			toIsBot = req.ToIsBot()
		}

		members, err := t.initializeDirectThreadDialogs(ctx, uow, createdThread.ID, req.DomainID, req.From, req.To, toIsBot)
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
		Via:        tde.Via,
	}
}

func (t *ThreadManagementService) initializeDirectThreadDialogs(ctx context.Context, uow store.UnitOfWork, threadID uuid.UUID, domainID int, from, to *shared.Peer, toIsBot bool) ([]*model.ThreadDialogExtended, error) {
	if from == nil || to == nil {
		return nil, errors.InvalidArgument("from and to peers is required", errors.WithID("service.thread_manager.initialize_direct_thread_dialogs"))
	}

	if uow == nil {
		return nil, errors.InvalidArgument("unit of work cannot be nil", errors.WithID("service.thread_manager.initialize_direct_thread_dialogs"))
	}

	if to.Identity == nil || from.Identity == nil {
		return nil, errors.InvalidArgument("from and to peers must have identity", errors.WithID("service.thread_manager.initialize_direct_thread_dialogs"))
	}

	if to.Identity.Name == "" || from.Identity.Name == "" {
		return nil, errors.InvalidArgument("from and to peers must have identity with non empty name", errors.WithID("service.thread_manager.initialize_direct_thread_dialogs"))
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

	baseModel := shared.BaseModel{DomainID: domainID}

	initiatorCreatedThreadDialog, err := uow.ThreadDialogStore().Create(ctx, &model.ThreadDialogExtended{
		BaseModel:   baseModel,
		ThreadID:    threadID,
		ContactID:   from.ID,
		ThreadRole:  initiatorRole,
		Permissions: *initiatorPermissions,
		Via:         from.ResolveVia(),
		Settings:    model.BaseThreadSetting{Title: to.Identity.Name},
	})
	if err != nil {
		return nil, err
	}

	targetCreatedThreadDialog, err := uow.ThreadDialogStore().Create(ctx, &model.ThreadDialogExtended{
		BaseModel:   baseModel,
		ThreadID:    threadID,
		ContactID:   to.ID,
		Via:         to.ResolveVia(),
		ThreadRole:  peerRole,
		Permissions: *targetPermissions,
		IsBot:       toIsBot,
		AutoLeave:   false,
		Settings:    model.BaseThreadSetting{Title: from.Identity.Name},
	})
	if err != nil {
		return nil, err
	}

	if toIsBot {
		t.logger.DebugContext(ctx, "create thread: target is bot, pushing bot control stack",
			"thread_id", threadID,
			"member_id", targetCreatedThreadDialog.ID,
		)

		pushResult, pushErr := uow.BotControl().Push(ctx, model.BotControlTransition{
			ThreadID:    threadID,
			NewMemberID: targetCreatedThreadDialog.ID,
			Reason:      model.BotControlReasonInitial,
		})
		if pushErr != nil {
			t.logger.ErrorContext(ctx, "create thread: failed to push bot control stack",
				"thread_id", threadID,
				"member_id", targetCreatedThreadDialog.ID,
				"err", pushErr,
			)
			return nil, errors.Internal("failed to init bot control", errors.WithCause(pushErr))
		}

		t.logger.DebugContext(ctx, "create thread: bot control stack pushed",
			"thread_id", threadID,
			"member_id", targetCreatedThreadDialog.ID,
			"control_epoch", pushResult.ControlEpoch,
		)

		if err = t.publishBotControlGranted(ctx, uow, targetCreatedThreadDialog, nil, 0, model.BotControlReasonInitial, false, pushResult.ControlEpoch); err != nil {
			return nil, err
		}
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

func (t *ThreadManagementService) publishMemberEvent(ctx context.Context, uow store.UnitOfWork, e ThreadEvent) error {
	return uow.Outbox().Publish(ctx, e.Topic(), e)
}

// positionAfterPush returns the stack position that was just pushed.
// prev is the entry that was top before the push (nil if stack was empty).
// botControlStackEntryToDialog builds a minimal ThreadDialogExtended from a BotControlStackEntry.
// Used after Pop to avoid an extra GetFullView round-trip when publishing granted events.
func botControlStackEntryToDialog(e *model.BotControlStackEntry) *model.ThreadDialogExtended {
	d := &model.ThreadDialogExtended{}
	d.ID = *e.MemberID
	d.DomainID = e.DomainID
	d.ThreadID = e.ThreadID
	d.ContactID = e.ContactID
	d.AutoLeave = e.AutoLeave

	return d
}

func positionAfterPush(prev *model.BotControlStackEntry) int {
	if prev == nil {
		return 0
	}

	return prev.Position + 1
}

// resolveAutoLeave returns true by default for bots unless explicitly overridden.
func resolveAutoLeave(override *bool) bool {
	if override != nil {
		return *override
	}

	return true
}

// publishBotControlGranted publishes a BotControlGranted event to the outbox.
// prev is the stack entry that was previously controlling (nil on initial activation).
// position is the new entry's stack position.
// isResume=true when returning control to a bot that was previously paused (Pop path).
// isResume=false when activating a newly added bot for the first time (Push path).
func (t *ThreadManagementService) publishBotControlGranted(ctx context.Context, uow store.UnitOfWork, dialog *model.ThreadDialogExtended, prev *model.BotControlStackEntry, position int, reason model.BotControlReason, isResume bool, controlEpoch int64) error {
	var (
		prevMemberID *uuid.UUID
		prevPosition *int
	)

	if prev != nil {
		prevMemberID = prev.MemberID
		prevPosition = &prev.Position
	}

	e := &event.BotControlGranted{
		ThreadID:         dialog.ThreadID,
		DomainID:         int32(dialog.DomainID),
		MemberID:         dialog.ID,
		ContactID:        dialog.ContactID,
		Position:         position,
		AutoLeave:        dialog.AutoLeave,
		Reason:           string(reason),
		IsResume:         isResume,
		PreviousPosition: prevPosition,
		PreviousMemberID: prevMemberID,
		ControlEpoch:     controlEpoch,
		OccurredAt:       time.Now().UTC(),
	}

	t.logger.DebugContext(ctx, "publishing bot.control.granted to outbox",
		"topic", e.Topic(),
		"thread_id", e.ThreadID,
		"member_id", e.MemberID,
		"control_epoch", e.ControlEpoch,
		"reason", e.Reason,
		"is_resume", e.IsResume,
	)

	if err := uow.Outbox().Publish(ctx, e.Topic(), e); err != nil {
		t.logger.ErrorContext(ctx, "failed to publish bot.control.granted to outbox",
			"topic", e.Topic(),
			"thread_id", e.ThreadID,
			"member_id", e.MemberID,
			"err", err,
		)
		return err
	}

	t.logger.DebugContext(ctx, "bot.control.granted published to outbox",
		"topic", e.Topic(),
		"thread_id", e.ThreadID,
		"member_id", e.MemberID,
		"control_epoch", e.ControlEpoch,
	)

	return nil
}

// publishBotControlReleased publishes a BotControlReleased event to the outbox.
func (t *ThreadManagementService) publishBotControlReleased(ctx context.Context, uow store.UnitOfWork, threadID, memberID uuid.UUID, position, domainID int, nextMemberID *uuid.UUID, reason model.BotControlReason) error {
	if memberID == uuid.Nil {
		return nil
	}

	e := &event.BotControlReleased{
		ThreadID:     threadID,
		DomainID:     int32(domainID),
		MemberID:     memberID,
		Position:     position,
		Reason:       string(reason),
		NextMemberID: nextMemberID,
		OccurredAt:   time.Now().UTC(),
	}

	t.logger.DebugContext(ctx, "publishing bot.control.released to outbox",
		"topic", e.Topic(),
		"thread_id", e.ThreadID,
		"member_id", e.MemberID,
		"reason", e.Reason,
		"next_member_id", e.NextMemberID,
	)

	if err := uow.Outbox().Publish(ctx, e.Topic(), e); err != nil {
		t.logger.ErrorContext(ctx, "failed to publish bot.control.released to outbox",
			"topic", e.Topic(),
			"thread_id", e.ThreadID,
			"member_id", e.MemberID,
			"err", err,
		)
		return err
	}

	t.logger.DebugContext(ctx, "bot.control.released published to outbox",
		"topic", e.Topic(),
		"thread_id", e.ThreadID,
		"member_id", e.MemberID,
	)

	return nil
}
