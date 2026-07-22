package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

// MessageStatusService applies per-recipient delivery confirmations reported
// by internal services (im-delivery: stream ACKs and pushes, im-providers:
// provider webhook receipts) and publishes message.status.v1 events for the
// statuses that actually changed.
type MessageStatusService struct {
	uow    store.UnitOfWork
	logger *slog.Logger
}

func NewMessageStatusService(uow store.UnitOfWork, logger *slog.Logger) *MessageStatusService {
	return &MessageStatusService{
		uow:    uow,
		logger: logger,
	}
}

func (s *MessageStatusService) MarkDelivered(ctx context.Context, receipts []*model.StatusReceipt) (int64, error) {
	var updated int64

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		changes, err := uow.MessageStatuses().MarkDelivered(txCtx, receipts)
		if err != nil {
			return err
		}

		updated = int64(len(changes))

		return dispatchStatusChangeEvents(txCtx, uow, changes)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "mark delivered failed", "err", err, "receipts", len(receipts))

		return 0, err
	}

	return updated, nil
}

func (s *MessageStatusService) MarkRead(ctx context.Context, receipts []*model.ReadReceipt) (int64, error) {
	var updated int64

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		changes, err := uow.MessageStatuses().MarkRead(txCtx, receipts)
		if err != nil {
			return err
		}

		updated = int64(len(changes))

		return dispatchStatusChangeEvents(txCtx, uow, changes)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "mark read failed", "err", err, "receipts", len(receipts))

		return 0, err
	}

	return updated, nil
}

func (s *MessageStatusService) MarkFailed(ctx context.Context, receipts []*model.StatusReceipt) (int64, error) {
	var updated int64

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context, uow store.UnitOfWork) error {
		changes, err := uow.MessageStatuses().MarkFailed(txCtx, receipts)
		if err != nil {
			return err
		}

		updated = int64(len(changes))

		return dispatchStatusChangeEvents(txCtx, uow, changes)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "mark failed failed", "err", err, "receipts", len(receipts))

		return 0, err
	}

	return updated, nil
}

// dispatchStatusChangeEvents publishes message.status.v1 events for status
// rows that actually changed, through the transactional outbox. Changes of
// the same (thread, member, status, via) are batched into a single event;
// failures stay one event per message to preserve individual error details.
// Events are stamped with current thread participants so im-delivery can
// fan them out without resolving the thread.
func dispatchStatusChangeEvents(ctx context.Context, uow store.UnitOfWork, changes []*model.StatusChange) error {
	events := groupStatusChanges(changes)
	if len(events) == 0 {
		return nil
	}

	participants, err := threadParticipants(ctx, uow, events)
	if err != nil {
		return err
	}

	for _, e := range events {
		e.Participants = participants[e.ThreadID]

		topic := fmt.Sprintf("im_message.%s.message.status.%s", e.RecipientID(), e.Version())

		if err := uow.Outbox().Publish(ctx, topic, e); err != nil {
			return fmt.Errorf("outbox publish failed: %w", err)
		}
	}

	return nil
}

// threadParticipants resolves member contact ids for every distinct thread
// mentioned by the events.
func threadParticipants(ctx context.Context, uow store.UnitOfWork, events []*event.MessageStatusChanged) (map[uuid.UUID][]uuid.UUID, error) {
	threadIDs := make([]uuid.UUID, 0, len(events))
	seen := make(map[uuid.UUID]struct{}, len(events))

	for _, e := range events {
		if _, ok := seen[e.ThreadID]; ok {
			continue
		}

		seen[e.ThreadID] = struct{}{}
		threadIDs = append(threadIDs, e.ThreadID)
	}

	dialogs, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
		ThreadIDs: threadIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("resolving thread participants: %w", err)
	}

	participants := make(map[uuid.UUID][]uuid.UUID, len(threadIDs))
	for _, d := range dialogs {
		if d == nil {
			continue
		}

		participants[d.ThreadID] = append(participants[d.ThreadID], d.ContactID)
	}

	return participants, nil
}

func groupStatusChanges(changes []*model.StatusChange) []*event.MessageStatusChanged {
	type key struct {
		threadID uuid.UUID
		memberID uuid.UUID
		status   model.MessageDeliveryStatus
		via      string
	}

	var (
		events  = make([]*event.MessageStatusChanged, 0, len(changes))
		grouped = make(map[key]*event.MessageStatusChanged, len(changes))
	)

	for _, c := range changes {
		if c == nil {
			continue
		}

		via := ""
		if c.Via != nil {
			via = *c.Via
		}

		e := &event.MessageStatusChanged{
			ThreadID:   c.ThreadID,
			DomainID:   c.DomainID,
			MemberID:   c.MemberID,
			MessageIDs: []uuid.UUID{c.MessageID},
			Status:     c.Status.String(),
			Via:        via,
			Error:      c.Error,
			OccurredAt: c.UpdatedAt,
		}

		// Failure events carry per-message error details, so they are not batched.
		if c.Status == model.MessageDeliveryStatusFailed {
			events = append(events, e)

			continue
		}

		k := key{c.ThreadID, c.MemberID, c.Status, via}
		if existing, ok := grouped[k]; ok {
			existing.MessageIDs = append(existing.MessageIDs, c.MessageID)
			if c.UpdatedAt.After(existing.OccurredAt) {
				existing.OccurredAt = c.UpdatedAt
			}

			continue
		}

		grouped[k] = e
		events = append(events, e)
	}

	return events
}
