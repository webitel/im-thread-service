package service

import (
	"context"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type threadVariables struct {
	store     store.ThreadVariablesStore
	logger    *slog.Logger
	publisher pubsub.EventPublisher
}

func NewThreadVariables(store store.ThreadVariablesStore, logger *slog.Logger, publisher pubsub.EventPublisher) *threadVariables {
	return &threadVariables{store: store, logger: logger, publisher: publisher}
}

func (tv *threadVariables) Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error) {
	log := tv.logger.With("operation", "service.thread_variables.set")

	result, err := tv.store.Set(ctx, variables)
	if err != nil {
		log.Error("failed to set thread variables", "error", err)
		return nil, err
	}

	brokerMessage, err := prepareVariablesMessage(ctx, result)
	if err != nil {
		log.Error("prepare set variables broker message", "error", err)
		return nil, err
	}

	if err := tv.publisher.Publish(result.PrepareSetTopic(variables.Member), brokerMessage); err != nil {
		log.Error("failed to publish thread variables", "error", err)

		return nil, errors.Internal(
			"publish set variables",
			errors.WithCause(err),
			errors.WithID("service.thread_variables.set"),
			errors.WithValue("thread_id", variables.Variables.ThreadID.String()),
		)
	}

	return result, nil
}

func (tv *threadVariables) Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error) {
	log := tv.logger.With("operation", "service.thread_variables.search")

	result, err := tv.store.Search(ctx, query)
	if err != nil {
		log.Error("search thread variables", "error", err)
		return model.Page[*model.ThreadVariables]{}, err
	}

	return result, nil
}

func (tv *threadVariables) Locate(ctx context.Context, threadID uuid.UUID) (*model.ThreadVariables, error) {
	log := tv.logger.With("operation", "service.thread_variables.locate")

	result, err := tv.store.Locate(ctx, threadID)
	if err != nil {
		log.Error("sql store locate", "error", err)
		return nil, err
	}

	return result, nil
}

func (tv *threadVariables) Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error) {
	log := tv.logger.With("operation", "service.thread_variables.flush")

	result, err := tv.store.Flush(ctx, flushCmd)
	if err != nil {
		log.Error("sql store flush", "error", err)
		return nil, err
	}

	brokerMessage, err := prepareVariablesMessage(ctx, result)
	if err != nil {
		log.Error("prepare flush variables broker message", "error", err)
		return nil, err
	}

	if err := tv.publisher.Publish(result.PrepareFlushTopic(flushCmd.Member), brokerMessage); err != nil {
		log.Error("publish thread variables", "error", err)

		return nil, errors.Internal(
			"publish flush variable event",
			errors.WithCause(err),
			errors.WithID("service.thread_variables.flush"),
			errors.WithValue("thread_id", flushCmd.ThreadID.String()),
		)
	}

	return result, nil
}

func prepareVariablesMessage(ctx context.Context, result *model.ThreadVariables) (*message.Message, error) {
	messagePayload, err := result.ToPayload()
	if err != nil {
		return nil, err
	}

	return message.NewMessageWithContext(
		ctx,
		uuid.Must(uuid.NewV7()).String(),
		messagePayload,
	), nil
}
