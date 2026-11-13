package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/store"
)

// retryPlan is what the read-only retry_of check decided before the send has
// any side effect. The in-transaction ReplaceFailedMessage stays the
// authoritative, race-safe check; this one only keeps a doomed retry from
// creating threads or processing media first.
type retryPlan struct {
	originalID uuid.UUID
	// threadID is the original's thread: the retry must land in it.
	threadID uuid.UUID
	// replayID is set when the original was already replaced by this actor: the
	// send is a network re-send and answers with that replacement.
	replayID uuid.UUID

	// actor is the real sender (never the send_as identity); actorIdentity
	// enriches the "replaced" event like a delete's deleter.
	actor         shared.Peer
	actorIdentity *model.ContactIdentity
}

func (p *retryPlan) replay() bool {
	return p != nil && p.replayID != uuid.Nil
}

// replayedMessage is the answer to a re-sent retry: the replacement that
// already exists, identified the way a fresh send's result is.
func (p *retryPlan) replayedMessage(req *model.Message) *model.Message {
	return &model.Message{
		ID:       p.replayID,
		ThreadID: p.threadID,
		DomainID: req.DomainID,
		From:     req.From,
		Type:     req.Type,
	}
}

// checkThread rejects a retry whose send resolved to another thread than the
// original's.
func (p *retryPlan) checkThread(threadID uuid.UUID) error {
	if p == nil || p.threadID == threadID {
		return nil
	}

	return errNotRetryable()
}

// errNotRetryable is the single answer for every retry_of that cannot be
// replaced, so a caller cannot probe other authors' messages through it.
func errNotRetryable() error {
	return errors.InvalidArgument(
		"retry_of must be a live message of yours in this chat that failed to send",
		errors.WithID("service.message.retry_of.not_retryable"),
	)
}

// planRetry checks retry_of against the original's current state: it must be a
// live, failed, retryable message the actor really authored. An original the
// same actor already replaced yields a replay plan. nil when retryOf is unset.
func (s *MessageService) planRetry(ctx context.Context, retryOf *uuid.UUID, actor shared.Peer, domainID int32) (*retryPlan, error) {
	if retryOf == nil || *retryOf == uuid.Nil {
		return nil, nil //nolint:nilnil // not a retry
	}

	target, err := s.uow.Messages().GetRetryTarget(ctx, *retryOf, domainID)
	if err != nil {
		return nil, err
	}

	if target == nil {
		return nil, errNotRetryable()
	}

	if target.RetriedBy != nil {
		if sameID(target.RetriedByAuthor, actor.ID) && sameID(target.RetriedByThread, target.ThreadID) {
			return &retryPlan{originalID: target.ID, threadID: target.ThreadID, replayID: *target.RetriedBy, actor: actor}, nil
		}

		return nil, errNotRetryable()
	}

	if target.Deleted || !target.Failed || target.AuthorID != actor.ID {
		return nil, errNotRetryable()
	}

	return &retryPlan{
		originalID:    target.ID,
		threadID:      target.ThreadID,
		actor:         actor,
		actorIdentity: s.resolveContactIdentity(ctx, actor.ID, domainID),
	}, nil
}

// replayAfterRace turns the loss of a race between two identical retries into
// the idempotent answer: when the transaction was refused because the original
// meanwhile got replaced by this very actor, the winner's message is returned.
func (s *MessageService) replayAfterRace(ctx context.Context, plan *retryPlan, txErr error, domainID int32) (uuid.UUID, bool) {
	if plan == nil || errors.Code(txErr) != codes.InvalidArgument {
		return uuid.Nil, false
	}

	again, err := s.planRetry(ctx, &plan.originalID, plan.actor, domainID)
	if err != nil || !again.replay() {
		return uuid.Nil, false
	}

	return again.replayID, true
}

// replaceRetried swaps the actor's failed message for its retry inside the
// send transaction: the original leaves the history and is announced deleted
// with replaced_by, so clients drop the bubble instead of tombstoning it. A
// retry_of that is not (or no longer) replaceable aborts the whole send.
func (s *MessageService) replaceRetried(ctx context.Context, uow store.UnitOfWork, plan *retryPlan, replacement *model.Message) error {
	if plan == nil {
		return nil
	}

	original, err := uow.Messages().ReplaceFailedMessage(ctx, plan.originalID, replacement.ID, replacement.ThreadID, plan.actor.ID)
	if err != nil {
		return err
	}

	// Announced like an ordinary delete: by the real actor, not the send_as
	// identity the replacement was sent as.
	original.To = replacement.To
	original.From = plan.actor
	original.WithReplacedEvent(ctx, plan.actorIdentity, replacement.ID)

	if err = s.dispatchMessageEvents(ctx, uow, original); err != nil {
		return err
	}

	// The original no longer counts as unread for anyone.
	return uow.MessageStatuses().RecountUnread(ctx, []uuid.UUID{replacement.ThreadID})
}

// logSendError logs a failed send; a request the client got wrong (a rejected
// retry_of, a bad payload) is not a service error.
func logSendError(ctx context.Context, log *slog.Logger, msg string, err error, attrs ...any) {
	attrs = append(attrs, slog.Any("error", err))

	if errors.Code(err) == codes.InvalidArgument {
		log.WarnContext(ctx, msg, attrs...)

		return
	}

	log.ErrorContext(ctx, msg, attrs...)
}

func sameID(id *uuid.UUID, want uuid.UUID) bool {
	return id != nil && *id == want
}
