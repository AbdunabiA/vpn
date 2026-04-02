package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient parses a Redis URL and returns a connected client.
// The URL must be in the form redis://[user:password@]host[:port][/db].
// An error is returned if the URL is malformed or the server is unreachable.
func NewRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Verify connectivity before returning.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return client, nil
}

// blacklistKeyPrefix is the Redis key namespace for blacklisted JWT access tokens.
const blacklistKeyPrefix = "token:blacklist:"

// IsTokenBlacklisted reports whether the given token hash is present in the
// blacklist. It returns false on any Redis error so callers fail open rather
// than locking out all users during a Redis outage.
func IsTokenBlacklisted(ctx context.Context, client *redis.Client, tokenHash string) bool {
	key := blacklistKeyPrefix + tokenHash
	val, err := client.Exists(ctx, key).Result()
	if err != nil {
		// Fail open — Redis unavailability must not block all authenticated traffic.
		return false
	}
	return val > 0
}

// BlacklistToken marks a token hash as revoked with the given TTL.
// After ttl elapses the key is automatically removed by Redis.
func BlacklistToken(ctx context.Context, client *redis.Client, tokenHash string, ttl time.Duration) error {
	key := blacklistKeyPrefix + tokenHash
	if err := client.Set(ctx, key, 1, ttl).Err(); err != nil {
		return fmt.Errorf("blacklisting token: %w", err)
	}
	return nil
}

// rateLimitKeyPrefix is the Redis key namespace for per-user/per-IP rate limits.
const rateLimitKeyPrefix = "rate:"

// IncrRateLimit increments the request counter for the given rate-limit key
// and sets the expiry to window on the first increment only, so the window
// does not reset on every request. Returns the current counter value after
// incrementing.
func IncrRateLimit(ctx context.Context, client *redis.Client, key string, window time.Duration) (int64, error) {
	fullKey := rateLimitKeyPrefix + key

	// INCR is atomic — pipeline it with nothing else so we get the count back
	// cleanly before deciding whether to set the expiry.
	pipe := client.Pipeline()
	incrCmd := pipe.Incr(ctx, fullKey)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("rate limit pipeline: %w", err)
	}

	count, err := incrCmd.Result()
	if err != nil {
		return 0, fmt.Errorf("reading incr result: %w", err)
	}

	// Only set the expiry on the very first increment so the window is fixed
	// from the moment the counter was created — subsequent increments within
	// the same window must not push the expiry forward.
	if count == 1 {
		if err := client.Expire(ctx, fullKey, window).Err(); err != nil {
			// Non-fatal: counter exists, expiry failed. Log externally if needed.
			// The key will be cleaned up by Redis eventually or on the next request.
			return count, fmt.Errorf("setting rate limit expiry: %w", err)
		}
	}

	return count, nil
}
