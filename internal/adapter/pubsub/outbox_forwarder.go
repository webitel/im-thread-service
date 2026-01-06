package pubsub

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
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

	router.AddHandler(
		"outbox_forwarder",
		"im.messages",
		outboxSub,
		"chat.events",
		rabbitPub,
		func(msg *message.Message) ([]*message.Message, error) {
			slog.Info("outbox -> rabbit",
				"message_uuid", msg.UUID,
				"payload", string(msg.Payload),
			)
			return []*message.Message{msg}, nil
		},
	)

	
	mainCtx, cancelMain := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			slog.Info("starting outbox forwarder leadership election")

			go elector.Run(mainCtx,
				func(leaderCtx context.Context) error {
					slog.Info("node promoted to LEADER: starting forwarder jobs")

					// 1. Start Cleanup Job (tied to leaderCtx)
					go StartOutboxCleanupJob(leaderCtx, outbox, slog)

					// 2. Start Watermill Router (tied to leaderCtx)
					// When leaderCtx is cancelled (leadership lost), router.Run returns
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
					// We don't necessarily need to close the router here if router.Run(leaderCtx)
					// is used, but it's good practice for an immediate stop.
				},
			)

			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down outbox forwarder")
			cancelMain() // Signal LeaderElector and all jobs to stop
			return router.Close()
		},
	})

	return nil
}

func StartOutboxCleanupJob(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	const cleanupInterval = 24 * time.Hour

	// Optional: initial run
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
	// Records older than 3 days
	n, err := outbox.Cleanup(ctx, 3)
	if err != nil {
		logger.Error("outbox cleanup: failed", "error", err)
		return
	}
	if n > 0 {
		logger.Info("outbox cleanup: successful", "deleted_count", n)
	}
}
