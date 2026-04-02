package middleware

import (
	"time"

	"vpnapp/server/api/internal/cache"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	authenticatedRateLimit   = 200
	unauthenticatedRateLimit = 30
	rateLimitWindow          = 1 * time.Minute
)

// RateLimit returns per-user rate-limit middleware backed by Redis.
//
// Authenticated requests (those that already have "user_id" in context locals,
// set by AuthRequired) are limited to authenticatedRateLimit req/min keyed on
// the user ID. Unauthenticated requests are limited to unauthenticatedRateLimit
// req/min keyed on the client IP.
//
// When Redis is unavailable IncrRateLimit returns an error and the request is
// allowed through — the middleware degrades gracefully rather than blocking
// all traffic during a Redis outage.
func RateLimit(redisClient *redis.Client, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var key string
		var limit int64

		userID, _ := c.Locals("user_id").(string)
		if userID != "" {
			key = "user:" + userID
			limit = authenticatedRateLimit
		} else {
			key = "ip:" + c.IP()
			limit = unauthenticatedRateLimit
		}

		count, err := cache.IncrRateLimit(c.Context(), redisClient, key, rateLimitWindow)
		if err != nil {
			// Log but do not block the request — Redis unavailability must not
			// cause a service outage.
			logger.Warn("rate limit check failed, allowing request",
				zap.String("key", key),
				zap.Error(err),
			)
			return c.Next()
		}

		if count > limit {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too many requests, please slow down",
			})
		}

		return c.Next()
	}
}
