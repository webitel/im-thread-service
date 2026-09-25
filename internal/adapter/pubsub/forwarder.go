// Package pubsub implements the Transactional Outbox Forwarder with Consul-based leadership.
// The journal is written in the mutation tx; this relay feeds live delivery only; lost events are caught up.
package pubsub

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/fx"

	leader "github.com/webitel/im-thread-service/infra/discovery/consul"
	"github.com/webitel/im-thread-service/internal/adapter/journal"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

const (
	ForwarderHandlerName = "webitel.im.outbox_forwarder"
	OutboxTopic          = "im.messages"
	DefaultFallbackTopic = "chat.events.v1"
	MetadataRoutingKey   = "x-routing-key"
	// ConsumerGroupName: watermill consumer group; cleanup must match or outbox grows forever.
	ConsumerGroupName = "im-thread-outbox-forwarder"
	PoisonQueueTopic  = "webitel.im.outbox.dead_letter"
)

// relayed counts outbox events handed to RabbitMQ, by result (ok|error).
var relayed = func() metric.Int64Counter {
	c, err := otel.Meter("github.com/webitel/im-thread-service/internal/adapter/pubsub").
		Int64Counter("im_thread_outbox_relayed_total", metric.WithDescription("Outbox events relayed to the broker, by result."))
	if err != nil {
		panic(err)
	}

	return c
}()

func RegisterOutboxForwarder(
	lc fx.Lifecycle,
	newSubscriber OutboxSubscriberFactory,
	jrnl *journal.Journal,
	rabbitPub EventPublisher,
	logger watermill.LoggerAdapter,
	elector leader.LeadershipElector,
	outbox store.OutboxStore,
	slog *slog.Logger,
) error {
	mainCtx, cancelMain := context.WithCancel(context.Background())
	electorDone := make(chan struct{})

	var routers sync.WaitGroup

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			slog.Info("starting webitel outbox forwarder with leadership election")

			// Only the active leader relays to prevent duplicate delivery.
			go func() {
				defer close(electorDone)

				elector.Run(mainCtx,
					func(leaderCtx context.Context) error {
						slog.Info("node PROMOTED to leader: initializing background workers")

						// A watermill router runs once, so each term needs a fresh one; reusing it
						// left a re-promoted node holding the lock while relaying nothing.
						router, err := newForwarderRouter(newSubscriber, rabbitPub, logger, slog)
						if err != nil {
							slog.Error("outbox forwarder: build router", "error", err)

							return err
						}

						go StartOutboxCleanupJob(leaderCtx, outbox, slog)
						go StartJournalCleanupJob(leaderCtx, jrnl, slog)

						routers.Add(1)

						go func() {
							defer routers.Done()

							if err := router.Run(leaderCtx); err != nil {
								slog.Error("watermill router: unexpected stop", "error", err)
							}

							if err := router.Close(); err != nil {
								slog.Warn("watermill router: close", "error", err)
							}
						}()

						return nil
					},
					func() {
						slog.Warn("node DEMOTED to follower: halting leader-specific tasks")
					},
				)
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down webitel outbox forwarder")
			cancelMain()

			// Let the elector release the Consul lock (else TTL expiry) and the router
			// finish in-flight acks before the pool and broker are closed.
			select {
			case <-electorDone:
			case <-ctx.Done():
				slog.Warn("timed out waiting for leader election to release the lock")
			}

			routersDone := make(chan struct{})

			go func() {
				routers.Wait()
				close(routersDone)
			}()

			select {
			case <-routersDone:
			case <-ctx.Done():
				slog.Warn("timed out waiting for the outbox router to stop")
			}

			return nil
		},
	})

	return nil
}

// newForwarderRouter builds one leadership term's router: middleware stack plus
// the single relay handler over a fresh outbox subscriber.
func newForwarderRouter(
	newSubscriber OutboxSubscriberFactory,
	rabbitPub EventPublisher,
	logger watermill.LoggerAdapter,
	log *slog.Logger,
) (*message.Router, error) {
	sub, err := newSubscriber()
	if err != nil {
		return nil, err
	}

	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return nil, err
	}

	router.AddMiddleware(middleware.Recoverer) // Prevent handler panics from crashing the service

	throttle := middleware.NewThrottle(100, time.Second)
	router.AddMiddleware(throttle.Middleware)

	router.AddMiddleware(middleware.Timeout(time.Second * 10))

	// Lost live events are recoverable: the journal holds them and the client
	// detects gaps and catches up.
	poisonHandler, err := middleware.PoisonQueue(rabbitPub, PoisonQueueTopic)
	if err != nil {
		return nil, err
	}

	router.AddMiddleware(poisonHandler)

	router.AddMiddleware(middleware.Retry{
		MaxRetries:      5,
		InitialInterval: time.Millisecond * 200,
		MaxInterval:     time.Second * 5,
		Multiplier:      2.0,
		Logger:          logger,
		OnRetryHook: func(retryNum int, delay time.Duration) {
			log.Warn("OUTBOX_FORWARDER_RETRY_TRIGGERED",
				"attempt", retryNum,
				"delay_ms", delay.Milliseconds(),
				"handler", ForwarderHandlerName,
			)
		},
	}.Middleware)

	router.AddConsumerHandler(
		ForwarderHandlerName,
		OutboxTopic,
		sub,
		func(msg *message.Message) error {
			return handleOutboxEvent(msg, rabbitPub, log)
		},
	)

	return router, nil
}

// handleOutboxEvent relays outbox message to RabbitMQ; publish errors trigger retry.
func handleOutboxEvent(msg *message.Message, pub EventPublisher, log *slog.Logger) error {
	topic := msg.Metadata.Get(MetadataRoutingKey)
	if topic == "" {
		topic = DefaultFallbackTopic
	}

	log.Info("forwarding outbox event",
		"topic", topic,
		"msg_uuid", msg.UUID,
		"correlation_id", middleware.MessageCorrelationID(msg))

	if err := pub.Publish(topic, msg); err != nil {
		relayed.Add(msg.Context(), 1, metric.WithAttributes(attribute.String("result", "error")))
		log.Error("failed to publish outbox event to rabbitmq",
			"topic", topic,
			"msg_uuid", msg.UUID,
			"err", err,
		)

		return err
	}

	relayed.Add(msg.Context(), 1, metric.WithAttributes(attribute.String("result", "ok")))
	log.Debug("outbox event published to rabbitmq",
		"topic", topic,
		"msg_uuid", msg.UUID,
	)

	return nil
}

// StartOutboxCleanupJob handles the scheduling of processed message removal.
func StartOutboxCleanupJob(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	const cleanupInterval = 24 * time.Hour

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

// StartJournalCleanupJob periodically trims the per-thread catch-up journal to
// its TTL window; clients offline past that window fall back to a resync.
func StartJournalCleanupJob(ctx context.Context, jrnl *journal.Journal, logger *slog.Logger) {
	const cleanupInterval = time.Hour

	doJournalCleanup(ctx, jrnl, logger)

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("journal cleanup job: context canceled")

			return
		case <-ticker.C:
			doJournalCleanup(ctx, jrnl, logger)
		}
	}
}

func doJournalCleanup(ctx context.Context, jrnl *journal.Journal, logger *slog.Logger) {
	n, err := jrnl.Cleanup(ctx)
	if err != nil {
		logger.Error("journal cleanup failed", "error", err)

		return
	}

	if n > 0 {
		logger.Info("journal cleanup successful", "deleted_count", n)
	}
}

// outboxCleanupOptions is the cleanup policy; ConsumerGroups must name the
// subscriber's real group (see ConsumerGroupName).
func outboxCleanupOptions() *model.OutboxCleanupOptions {
	return &model.OutboxCleanupOptions{
		RetentionDays:  3,
		BatchSize:      5000,
		ConsumerGroups: []string{ConsumerGroupName},
		Topic:          OutboxTopic,
	}
}

// doCleanup triggers the physical deletion of acknowledged outbox records.
func doCleanup(ctx context.Context, outbox store.OutboxStore, logger *slog.Logger) {
	n, err := outbox.Cleanup(ctx, outboxCleanupOptions())
	if err != nil {
		logger.Error("outbox cleanup failed", "error", err)

		return
	}

	if n > 0 {
		logger.Info("outbox cleanup successful", "deleted_count", n)
	}
}
