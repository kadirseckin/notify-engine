package ratelimiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"notify-engine/internal/telemetry"
)

type RateLimiter interface {
	Allow(ctx context.Context, channel string) (allowed bool, waitHint time.Duration, err error)
}

type redisRateLimiter struct {
	client    *redis.Client
	maxPerSec int
	script    *redis.Script
}

func NewRedisRateLimiter(client *redis.Client, maxPerSec int) RateLimiter {
	script := redis.NewScript(`
		local key = KEYS[1]
		local limit = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local current = redis.call('INCR', key)
		if current == 1 then
			redis.call('PEXPIRE', key, window)
		end
		if current <= limit then
			return 1
		end
		return 0
	`)
	return &redisRateLimiter{client: client, maxPerSec: maxPerSec, script: script}
}

func (r *redisRateLimiter) Allow(ctx context.Context, channel string) (bool, time.Duration, error) {
	ctx, span := otel.Tracer(telemetry.Name).Start(ctx, "ratelimiter.allow")
	defer span.End()
	span.SetAttributes(attribute.String("ratelimiter.channel", channel))

	now := time.Now()
	key := fmt.Sprintf("ratelimit:%s:%d", channel, now.Unix())
	result, err := r.script.Run(ctx, r.client, []string{key}, r.maxPerSec, 1100).Int()
	if err != nil {
		return false, 0, fmt.Errorf("rate limiter: %w", err)
	}
	if result == 1 {
		span.SetAttributes(attribute.Bool("ratelimiter.allowed", true))
		return true, 0, nil
	}

	nextWindow := now.Truncate(time.Second).Add(time.Second)
	span.SetAttributes(attribute.Bool("ratelimiter.allowed", false))
	return false, time.Until(nextWindow), nil
}
