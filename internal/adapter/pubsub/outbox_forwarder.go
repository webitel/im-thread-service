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
			return []*message.Message{msg}, nil
		},
	)

	ctx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go elector.Run(ctx,
				func(leaderCtx context.Context) error {
					go StartOutboxCleanupJob(leaderCtx, outbox, slog)
					return router.Run(leaderCtx)
				},
				func() { _ = router.Close() },
			)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return router.Close()
		},
	})

	return nil
}

func StartOutboxCleanupJob(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := outbox.Cleanup(ctx, 3)
			if err != nil {
				logger.Error("outbox cleanup failed", slog.Any("error", err))
			} else if n > 0 {
				logger.Info("outbox cleaned", slog.Int64("count", n))
			}
		}
	}
}
