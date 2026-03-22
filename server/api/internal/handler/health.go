package handler

import (
	"errors"
	"runtime"
	"time"

	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var startTime = time.Now()

// Health handles GET /health.
func Health() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "healthy",
			"uptime":     time.Since(startTime).Round(time.Second).String(),
			"go_version": runtime.Version(),
			"timestamp":  time.Now().UTC(),
		})
	}
}

// GetSubscription handles GET /subscription.
// Returns the user's active subscription from the database.
func GetSubscription(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)

		sub, err := repository.FindSubscriptionByUserID(db, userID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				// No subscription found — return default free plan
				return c.JSON(fiber.Map{
					"data": fiber.Map{
						"plan":        "free",
						"is_active":   true,
						"max_devices": model.PlanLimits["free"].MaxDevices,
					},
				})
			}
			logger.Error("failed to get subscription", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		limits := model.PlanLimits[sub.Plan]

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"id":          sub.ID,
				"plan":        sub.Plan,
				"is_active":   sub.IsActive,
				"started_at":  sub.StartedAt,
				"expires_at":  sub.ExpiresAt,
				"max_devices": limits.MaxDevices,
			},
		})
	}
}

// GetAccount handles GET /account.
// Returns the user's account information from the database.
func GetAccount(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)

		user, err := repository.FindUserByID(db, userID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "user not found",
				})
			}
			logger.Error("failed to get user", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"id":                      user.ID,
				"subscription_tier":       user.SubscriptionTier,
				"subscription_expires_at": user.SubscriptionExpiresAt,
				"created_at":              user.CreatedAt,
			},
		})
	}
}

// ErrorHandler returns a custom Fiber error handler that logs errors.
func ErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		logger.Error("request error",
			zap.Int("status", code),
			zap.String("path", c.Path()),
			zap.Error(err),
		)

		return c.Status(code).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
}
