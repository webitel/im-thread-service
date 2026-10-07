package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

const maxPreviewDuration = time.Hour

type (
	ThreadPreviewService struct {
		uow     store.UnitOfWork
		logger  *slog.Logger
		members previewMemberAdder
	}

	previewMemberAdder interface {
		addMember(ctx context.Context, req *dto.AddMemberRequest, beforeCreate func(context.Context, store.UnitOfWork) error) (uuid.UUID, error)
	}
)

func NewThreadPreviewService(logger *slog.Logger, uow store.UnitOfWork, members *ThreadManagementService) *ThreadPreviewService {
	if logger == nil {
		logger = slog.Default()
	}

	return &ThreadPreviewService{
		uow:     uow,
		logger:  logger.With(slog.String("component", "thread_preview")),
		members: members,
	}
}

func (s *ThreadPreviewService) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}

	return slog.Default()
}

// validatePreviewTarget validates that thread, contact, initiator IDs are not nil
// and that domainID is positive.
func validatePreviewTarget(threadID, contactID, initiatorID uuid.UUID, domainID int, op string) error {
	if threadID == uuid.Nil || contactID == uuid.Nil {
		return errors.InvalidArgument("thread_id and contact_id must not be empty", errors.WithID("service.thread_preview."+op))
	}

	if domainID <= 0 {
		return errors.InvalidArgument("domain_id must be positive", errors.WithID("service.thread_preview."+op))
	}

	if initiatorID == uuid.Nil {
		return errors.InvalidArgument("initiator_contact_id must not be empty", errors.WithID("service.thread_preview."+op))
	}

	return nil
}

func (s *ThreadPreviewService) GivePreview(ctx context.Context, req *dto.GivePreviewRequest) (*model.ThreadPreview, error) {
	if err := validatePreviewTarget(req.ThreadID, req.ContactID, req.InitiatorContactID, req.DomainID, "give"); err != nil {
		return nil, err
	}

	if req.Duration <= 0 || req.Duration > maxPreviewDuration {
		return nil, errors.InvalidArgument("duration must be positive and at most 1h", errors.WithID("service.thread_preview.give"))
	}

	var result *model.ThreadPreview

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		// Lock the thread to serialize GivePreview/Upgrade calls per thread
		if err := uow.ThreadStore().LockForUpdate(ctx, req.ThreadID); err != nil {
			return err
		}

		// Check if contact is already an active member
		members, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs:  []uuid.UUID{req.ThreadID},
			ContactIDs: []uuid.UUID{req.ContactID},
		})
		if err != nil {
			return err
		}

		if len(members) > 0 {
			return errors.New("contact is already a member of the thread", errors.WithCode(codes.AlreadyExists), errors.WithID("service.thread_preview.give.already_member"))
		}

		// Extends the active preview or starts a new one; the unique index keeps one active per pair.
		result, err = uow.ThreadPreviews().Upsert(ctx, &model.ThreadPreview{
			DomainID:    req.DomainID,
			ThreadID:    req.ThreadID,
			ContactID:   req.ContactID,
			InitiatorID: req.InitiatorContactID,
		}, req.Duration)

		return err
	})
	if err != nil {
		return nil, err
	}

	s.log().InfoContext(ctx, "preview given",
		"operation", "service.thread_preview.give",
		"thread_id", req.ThreadID,
		"contact_id", req.ContactID,
		"initiator_id", req.InitiatorContactID,
		"expires_at", result.ExpiresAt,
	)

	return result, nil
}

func (s *ThreadPreviewService) RemoveFromPreview(ctx context.Context, req *dto.RemoveFromPreviewRequest) error {
	if err := validatePreviewTarget(req.ThreadID, req.ContactID, uuid.Nil, req.DomainID, "remove"); err != nil {
		// Special case: initiator is optional, so we don't validate it here
		if req.ContactID == uuid.Nil || req.ThreadID == uuid.Nil || req.DomainID <= 0 {
			return errors.InvalidArgument("thread_id and contact_id must not be empty, domain_id must be positive", errors.WithID("service.thread_preview.remove"))
		}
	}

	err := s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		active, err := uow.ThreadPreviews().GetActiveForUpdate(ctx, req.ThreadID, req.ContactID, req.DomainID)
		if err != nil {
			if errors.Is(err, store.ErrPreviewNotActive) {
				// Idempotent: no active preview, return nil
				return nil
			}

			return err
		}

		err = uow.ThreadPreviews().Revoke(ctx, active.ID, model.PreviewRevokeReasonRemove)
		if err != nil {
			if errors.Is(err, store.ErrPreviewNotActive) {
				// Concurrent revoke, idempotent
				return nil
			}

			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	s.log().InfoContext(ctx, "preview removed",
		"operation", "service.thread_preview.remove",
		"thread_id", req.ThreadID,
		"contact_id", req.ContactID,
		"initiator_id", req.InitiatorContactID,
	)

	return nil
}

func errPreviewNotActive() error {
	return errors.New("preview has expired or was revoked", errors.WithCode(codes.FailedPrecondition), errors.WithID("service.thread_preview.upgrade.not_active"))
}

func (s *ThreadPreviewService) UpgradePreviewToFullMember(ctx context.Context, req *dto.UpgradePreviewRequest) (uuid.UUID, error) {
	if err := validatePreviewTarget(req.ThreadID, req.ContactID, req.InitiatorContactID, req.DomainID, "upgrade"); err != nil {
		return uuid.Nil, err
	}

	if req.Role == model.UnspecifiedRole {
		return uuid.Nil, errors.InvalidArgument("unknown member role", errors.WithID("service.thread_preview.upgrade.role"))
	}

	if _, err := getDefaultPermissionsByRole(req.Role); err != nil {
		return uuid.Nil, errors.InvalidArgument("unknown member role", errors.WithID("service.thread_preview.upgrade.role"))
	}

	return s.members.addMember(ctx, &dto.AddMemberRequest{
		ThreadID:           req.ThreadID,
		NewMemberContactID: req.ContactID,
		InitiatorContactID: req.InitiatorContactID,
		NewMemberRole:      req.Role,
		DomainID:           req.DomainID,
		SystemCall:         true, // trusted orchestrator: initiator role not checked
	}, func(ctx context.Context, uow store.UnitOfWork) error {
		p, err := uow.ThreadPreviews().GetActiveForUpdate(ctx, req.ThreadID, req.ContactID, req.DomainID)
		if errors.Is(err, store.ErrPreviewNotActive) {
			return errPreviewNotActive()
		}

		if err != nil {
			return err
		}

		if err := uow.ThreadPreviews().Revoke(ctx, p.ID, model.PreviewRevokeReasonUpgrade); err != nil {
			if errors.Is(err, store.ErrPreviewNotActive) {
				return errPreviewNotActive()
			}

			return err
		}

		return nil
	})
}
