package server

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/webitel/webitel-go-kit/pkg/cache"

	"github.com/webitel/im-thread-service/config"
	adapterpubsub "github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"github.com/webitel/im-thread-service/internal/adapter/ratelimit"
	"github.com/webitel/im-thread-service/internal/service"
)

// ProvideTypingConfig exposes the typing settings as a standalone injectable
// value so the service layer does not depend on the whole *config.Config.
func ProvideTypingConfig(cfg *config.Config) config.TypingConfig {
	return cfg.Typing
}

// ProvideRedisClient builds the shared Redis client from config and closes it
// on shutdown. Redis backs the ephemeral typing rate limiter.
func ProvideRedisClient(lc fx.Lifecycle, cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	lc.Append(fx.Hook{
		OnStop: func(context.Context) error { return rdb.Close() },
	})

	return rdb
}

// ProvideTypingRateLimiter builds the TTL-bounded typing rate limiter. It uses
// the shared cache with an L2 (Redis) layer only: the limit must be consistent
// across instances, so a per-instance L1 layer would be wrong here.
func ProvideTypingRateLimiter(rdb *redis.Client, cfg *config.Config, logger *slog.Logger) (service.RateLimiter, error) {
	c, err := cache.New[string, string]().
		Name("typing-ratelimit").
		L2(cache.RedisConfig[string]{
			Client: rdb,
			Prefix: "typing:rl",
			TTL:    cfg.Typing.RateLimitWindow,
			Codec:  cache.RawString(),
		}).
		Build()
	if err != nil {
		return nil, err
	}

	return ratelimit.New(c, cfg.Typing.RateLimitWindow, logger), nil
}

// provideTypingBus bridges the ephemeral RabbitMQ publisher (which bypasses the
// outbox) to the service-layer TypingBus port.
func provideTypingBus(p adapterpubsub.EventPublisher) service.TypingBus {
	return p
}
