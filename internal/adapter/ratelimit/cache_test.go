package ratelimit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mapCache is an in-memory cache.Cache[string,string] with no TTL expiry — good
// enough to exercise the limiter's arm/drop logic without a live Redis.
type mapCache struct {
	store  map[string]string
	getErr error
	setErr error
}

func newMapCache() *mapCache { return &mapCache{store: map[string]string{}} }

func (c *mapCache) Get(_ context.Context, key string) (string, bool, error) {
	if c.getErr != nil {
		return "", false, c.getErr
	}

	v, ok := c.store[key]

	return v, ok, nil
}

func (c *mapCache) Set(ctx context.Context, key, value string) error {
	return c.SetTTL(ctx, key, value, 0)
}

func (c *mapCache) SetTTL(_ context.Context, key, value string, _ time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}

	c.store[key] = value

	return nil
}

func (c *mapCache) Delete(_ context.Context, key string) error {
	delete(c.store, key)

	return nil
}

func (c *mapCache) Close() error { return nil }

func newTestLimiter(c *mapCache) *Limiter {
	return New(c, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestLimiter_FirstAllowedSecondDropped(t *testing.T) {
	l := newTestLimiter(newMapCache())
	ctx := context.Background()

	require.True(t, l.Allow(ctx, "k"), "first event in the window is allowed")
	require.False(t, l.Allow(ctx, "k"), "second event in the same window is dropped")
	require.True(t, l.Allow(ctx, "other"), "a different key is independent")
}

func TestLimiter_FailOpenOnGetError(t *testing.T) {
	c := newMapCache()
	c.getErr = errors.New("redis down")

	l := newTestLimiter(c)

	require.True(t, l.Allow(context.Background(), "k"), "cache error must fail open")
}

func TestLimiter_FailOpenOnSetError(t *testing.T) {
	c := newMapCache()
	c.setErr = errors.New("redis down")

	l := newTestLimiter(c)

	// Get is a miss, Set fails — the event still proceeds (fail open).
	require.True(t, l.Allow(context.Background(), "k"))
}
