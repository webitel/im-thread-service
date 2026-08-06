// Package ratelimit provides a best-effort, TTL-bounded rate limiter backed by
// the shared webitel-go-kit cache. It is used to throttle ephemeral typing
// events (see internal/service/typing.go).
package ratelimit

import (
	"context"
	"log/slog"
	"time"

	"github.com/webitel/webitel-go-kit/pkg/cache"
)

// Limiter throttles events per key to at most one per window.
//
// It is intentionally NOT atomic: cache.Cache exposes no compare-and-set, so a
// rare check-then-set race can let two events through in the same window. That
// is harmless for ephemeral typing. Any cache error fails OPEN (Allow returns
// true) — dropping a typing event is only an optimization and a cache blip must
// not break the indicator.
type Limiter struct {
	cache  cache.Cache[string, string]
	window time.Duration
	logger *slog.Logger
}

// New builds a Limiter over the given cache. The window must match the cache
// entry TTL so an armed key expires exactly one window after it was set.
func New(c cache.Cache[string, string], window time.Duration, logger *slog.Logger) *Limiter {
	return &Limiter{cache: c, window: window, logger: logger}
}

// Allow reports whether an event for key may proceed. The first caller within a
// window is allowed and arms the window; subsequent callers are dropped until
// the entry expires.
func (l *Limiter) Allow(ctx context.Context, key string) bool {
	if _, armed, err := l.cache.Get(ctx, key); err != nil {
		l.logger.WarnContext(ctx, "typing rate-limiter get failed, allowing", "err", err)

		return true // fail open
	} else if armed {
		return false // already seen within the window
	}

	if err := l.cache.SetTTL(ctx, key, "1", l.window); err != nil {
		l.logger.WarnContext(ctx, "typing rate-limiter set failed, allowing", "err", err)
	}

	return true
}
