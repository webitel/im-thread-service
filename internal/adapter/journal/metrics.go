package journal

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Journal instruments bind to the global MeterProvider; without the OTel SDK they are no-ops.
var (
	meter = otel.Meter("github.com/webitel/im-thread-service/internal/adapter/journal")

	resyncs = mustCounter("im_thread_updates_resyncs_total",
		"GetUpdates answers that told the client to reload its thread list, by reason.")

	cleanedUp = mustCounter("im_thread_journal_cleanup_deleted_total",
		"Journal rows deleted by retention.")
)

// Resync reasons, used as the metric label.
const (
	ResyncFirstSync = "first_sync"
	ResyncTrimmed   = "trimmed"
	ResyncTooMany   = "too_many"
)

// CountResync records a GetUpdates resync.
func CountResync(ctx context.Context, reason string) {
	resyncs.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func mustCounter(name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}

	return c
}
