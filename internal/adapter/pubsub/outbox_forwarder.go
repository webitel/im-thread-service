package pubsub

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
	"go.uber.org/fx"
)

type LeadershipElector interface {
	Run(ctx context.Context, onStart func(ctx context.Context) error, onStop func())
}

func RegisterOutboxForwarder(
	lc fx.Lifecycle,
	outboxSub OutboxSubscriber,
	rabbitPub EventPublisher,
	logger watermill.LoggerAdapter,
	elector LeadershipElector,
	outbox store.OutboxStore,
	slog *slog.Logger,
) error {
	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return err
	}

	// [HANDLER_CONFIG]
	// Move messages from local DB outbox to external RabbitMQ exchange
	router.AddConsumerHandler(
		"outbox_forwarder",
		"im.messages", // Читаємо з БД
		outboxSub,
		func(msg *message.Message) error {
			// [DYNAMIC_TOPIC]
			topic := msg.Metadata.Get("x-routing-key")
			if topic == "" {
				topic = "chat.events" // Fallback
			}

			slog.Info("forwarding message to dynamic topic",
				"topic", topic,
				"msg_uuid", msg.UUID)

			return rabbitPub.Publish(topic, msg)
		},
	)

	mainCtx, cancelMain := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			slog.Info("starting outbox forwarder leadership election")

			// Only the leader node handles outbox forwarding to avoid message duplication
			go elector.Run(mainCtx,
				func(leaderCtx context.Context) error {
					slog.Info("node promoted to LEADER: starting forwarder jobs")

					// 1. [HOUSEKEEPING]
					// Start periodic cleanup of acknowledged messages
					go StartOutboxCleanupJob(leaderCtx, outbox, slog)

					// 2. [FORWARDING]
					// Lifecycle of the router is bound to leaderCtx
					go func() {
						slog.Info("watermill router: starting")
						if err := router.Run(leaderCtx); err != nil {
							slog.Error("watermill router: stopped", "error", err)
						}
					}()

					return nil
				},
				func() {
					slog.Warn("node demoted to FOLLOWER: stopping leader-specific tasks")
				},
			)

			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down outbox forwarder")
			cancelMain() // Graceful stop for LeaderElector and workers
			return router.Close()
		},
	})

	return nil
}

func StartOutboxCleanupJob(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	const cleanupInterval = 24 * time.Hour

	// [INITIAL_CLEANUP]
	doCleanup(ctx, outbox, logger)

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("outbox cleanup job: context cancelled, stopping")
			return
		case <-ticker.C:
			doCleanup(ctx, outbox, logger)
		}
	}
}

func doCleanup(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	// [CLEANUP_STRATEGY]
	// Remove messages that are older than 3 days AND already acknowledged by the consumer.
	// Batching is handled internally by the store implementation.
	n, err := outbox.Cleanup(ctx, &model.OutboxCleanupOptions{
		RetentionDays: 3,
		BatchSize:     5000,
		ConsumerGroup: "im-thread-outbox-forwarder",
		Topic:         "im.messages",
	})
	if err != nil {
		logger.Error("outbox cleanup: failed", "error", err)
		return
	}

	if n > 0 {
		logger.Info("outbox cleanup complete", "deleted_count", n)
	}
}
