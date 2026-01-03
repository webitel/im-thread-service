package pubsub

import (
	"context"
	"log"
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

	router.AddMiddleware(OutboxMarkAsPublished(outbox))

	ctx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go elector.Run(ctx,
				func(leaderCtx context.Context) error {
					go StartOutboxCleanupJob(ctx, outbox, 30*time.Minute)
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

func StartOutboxCleanupJob(ctx context.Context, outbox store.OutboxStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := outbox.Cleanup(ctx, 1000)
			if err != nil {
				log.Printf("Outbox cleanup failed: %v", err)
			} else if n > 0 {
				log.Printf("Outbox cleaned %d messages", n)
			}
		}
	}
}
