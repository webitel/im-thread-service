// Package pubsub implements the High-Availability Transactional Outbox Forwarder.
//
// ARCHITECTURE:
// This component bridges the gap between persistent SQL storage and distributed message brokers (RabbitMQ).
// It ensures AT-LEAST-ONCE delivery semantics by relaying staged domain events from the 'messages_outbox'
// table to external exchanges.
//
// KEY FEATURES:
// 1. [LEADERSHIP] Integrated with Consul to ensure only one node (LEADER) performs the relay.
// 2. [RESILIENCE] Uses Poison Queue (DLQ) and Retry strategies to prevent Head-of-Line blocking.
// 3. [ROUTING] Supports dynamic, hierarchical routing keys (im_message.id.event.action.v1).
// 4. [CLEANUP] Automated background purging of acknowledged messages to prevent DB bloat.
package pubsub

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"go.uber.org/fx"

	"github.com/webitel/webitel-go-kit/pkg/semconv"

	leader "github.com/webitel/im-thread-service/infra/discovery/consul"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

// [CONSTANTS] Technical identifiers for infrastructure components
const (
	ForwarderHandlerName = "webitel.im.outbox_forwarder"
	OutboxTopic          = "im.messages"
	DefaultFallbackTopic = "chat.events.v1"
	MetadataRoutingKey   = "x-routing-key"
	ConsumerGroupName    = "webitel.im-thread-outbox-forwarder"
	PoisonQueueTopic     = "webitel.im.outbox.dead_letter" // [DLX_CONFIG] Target topic for exhausted retries or unrecoverable errors
)

func RegisterOutboxForwarder(
	lc fx.Lifecycle,
	outboxSub OutboxSubscriber,
	rabbitPub EventPublisher,
	logger watermill.LoggerAdapter,
	elector leader.LeadershipElector,
	outbox store.OutboxStore,
	slog *slog.Logger,
) error {
	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return err
	}

	// [STABILITY] Core middleware for resilience and observability
	router.AddMiddleware(middleware.Recoverer) // Prevent service crash on handler panic

	throttle := middleware.NewThrottle(100, time.Second) // 100 msg/sec
	router.AddMiddleware(throttle.Middleware)

	// [TIMEOUT] Ensure handlers don't hang indefinitely (e.g., during network partitions)
	router.AddMiddleware(middleware.Timeout(time.Second * 10))

	// [POISON_QUEUE] Final destination for unprocessable messages.
	poisonHandler, err := middleware.PoisonQueue(rabbitPub, PoisonQueueTopic)
	if err != nil {
		return err
	}

	router.AddMiddleware(poisonHandler)

	// [RETRY] Configurable backoff strategy for transient failures
	router.AddMiddleware(middleware.Retry{
		MaxRetries:      5,
		InitialInterval: time.Millisecond * 200,
		MaxInterval:     time.Second * 5,
		Multiplier:      2.0,
		Logger:          logger,
		// [RETRY_HOOK]
		OnRetryHook: func(retryNum int, delay time.Duration) {
			slog.Warn("OUTBOX_FORWARDER_RETRY_TRIGGERED",
				"attempt", retryNum,
				"delay_ms", delay.Milliseconds(),
				"handler", ForwarderHandlerName,
			)
		},
	}.Middleware)

	// [RELAY_LOGIC] Transfer messages from persistent DB outbox to RabbitMQ exchange
	router.AddConsumerHandler(
		ForwarderHandlerName,
		OutboxTopic,
		outboxSub,
		func(msg *message.Message) error {
			// [DYNAMIC_ROUTING] Resolve target topic from message metadata
			topic := msg.Metadata.Get(MetadataRoutingKey)
			if topic == "" {
				topic = DefaultFallbackTopic
			}

			slog.Info("forwarding outbox event",
				"topic", topic,
				"msg_uuid", msg.UUID,
				"correlation_id", middleware.MessageCorrelationID(msg))

			return rabbitPub.Publish(topic, msg)
		},
	)

	mainCtx, cancelMain := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			slog.Info("starting webitel outbox forwarder with leadership election")

			// [LEADERSHIP] Only the active Leader node performs outbox relay to prevent DUPLICATE delivery
			go elector.Run(mainCtx,
				func(leaderCtx context.Context) error {
					slog.Info("node PROMOTED to leader: initializing background workers")

					// [CLEANUP] Start periodic purging of processed outbox entries
					go StartOutboxCleanupJob(leaderCtx, outbox, slog)

					// [ROUTER] Run the Watermill message router bound to Leader context
					go func() {
						if err := router.Run(leaderCtx); err != nil {
							slog.Error("watermill router: unexpected stop", semconv.ErrorKey, err)
						}
					}()

					return nil
				},
				func() {
					slog.Warn("node DEMOTED to follower: halting leader-specific tasks")
				},
			)

			return nil
		},
		OnStop: func(_ context.Context) error {
			slog.Info("shutting down webitel outbox forwarder")
			cancelMain() // Signal LeaderElector and workers to stop

			return router.Close()
		},
	})

	return nil
}

// StartOutboxCleanupJob handles the scheduling of processed message removal.
func StartOutboxCleanupJob(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	const cleanupInterval = 24 * time.Hour

	// [INITIAL_PURGE] Execute cleanup immediately upon leader promotion
	doCleanup(ctx, outbox, logger)

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("outbox cleanup job: context canceled")

			return
		case <-ticker.C:
			doCleanup(ctx, outbox, logger)
		}
	}
}

// doCleanup triggers the physical deletion of acknowledged outbox records.
func doCleanup(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	// [STRATEGY] Batch remove messages older than 3 days acknowledged by ConsumerGroupName
	n, err := outbox.Cleanup(ctx, &model.OutboxCleanupOptions{
		RetentionDays: 3,
		BatchSize:     5000,
		ConsumerGroup: ConsumerGroupName,
		Topic:         OutboxTopic,
	})
	if err != nil {
		logger.Error("outbox cleanup failed", semconv.ErrorKey, err)

		return
	}

	if n > 0 {
		logger.Info("outbox cleanup successful", "deleted_count", n)
	}
}
