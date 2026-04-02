package handler

import (
	"errors"

	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServerConfig is the connection configuration sent to clients.
type ServerConfig struct {
	ServerAddress string               `json:"server_address"`
	ServerPort    int                  `json:"server_port"`
	Protocol      string               `json:"protocol"`
	UserID        string               `json:"user_id"`
	Reality       *RealityClientConfig `json:"reality,omitempty"`
}

// RealityClientConfig holds REALITY settings for the client.
type RealityClientConfig struct {
	PublicKey   string `json:"public_key"`
	ShortID     string `json:"short_id"`
	ServerName  string `json:"server_name"`
	Fingerprint string `json:"fingerprint"`
}

// ListServers handles GET /servers.
// Returns active VPN servers from the database, limited by subscription tier.
func ListServers(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)
		tier := c.Locals("tier").(string)

		servers, err := repository.ListActiveServers(db)
		if err != nil {
			logger.Error("failed to list servers", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Apply tier-based limit: free users see fewer servers
		if limits, ok := model.PlanLimits[tier]; ok && limits.MaxServers > 0 {
			if len(servers) > limits.MaxServers {
				servers = servers[:limits.MaxServers]
			}
		}

		logger.Debug("listing servers",
			zap.String("user_id", userID),
			zap.String("tier", tier),
			zap.Int("count", len(servers)),
		)

		return c.JSON(fiber.Map{
			"data": servers,
		})
	}
}

// GetServerConfig handles GET /servers/:id/config.
// Returns the connection configuration for a specific server.
// Enforces the plan's MaxDevices limit: returns 429 if the user is already at capacity.
func GetServerConfig(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		userID := c.Locals("user_id").(string)
		tier := c.Locals("tier").(string)

		server, err := repository.FindServerByID(db, serverID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "server not found",
				})
			}
			logger.Error("failed to find server", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		if !server.IsActive {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "server unavailable",
			})
		}

		// Enforce device limit before issuing a configuration.
		limits, ok := model.PlanLimits[tier]
		if !ok {
			limits = model.PlanLimits["free"]
		}

		activeCount, err := repository.CountActiveConnections(db, userID)
		if err != nil {
			logger.Error("failed to count active connections",
				zap.String("user_id", userID),
				zap.Error(err),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		if int(activeCount) >= limits.MaxDevices {
			logger.Warn("device limit reached at config fetch",
				zap.String("user_id", userID),
				zap.String("tier", tier),
				zap.Int64("active", activeCount),
				zap.Int("limit", limits.MaxDevices),
			)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":        "device limit reached",
				"active_count": activeCount,
				"max_devices":  limits.MaxDevices,
			})
		}

		logger.Debug("serving server config",
			zap.String("server_id", serverID),
			zap.String("user_id", userID),
		)

		// Build config with real REALITY keys from the database
		publicKey := server.RealityPublicKey
		shortID := server.RealityShortID
		if publicKey == "" {
			publicKey = "OAmaJn5JqNlYdNIulgafHAwZs8MLLuU8MXs9rt26sl0"
		}
		if shortID == "" {
			shortID = "abcd1234"
		}

		config := ServerConfig{
			ServerAddress: server.IPAddress,
			ServerPort:    443,
			Protocol:      server.Protocol,
			UserID:        userID,
			Reality: &RealityClientConfig{
				PublicKey:   publicKey,
				ShortID:     shortID,
				ServerName:  "www.microsoft.com",
				Fingerprint: "chrome",
			},
		}

		return c.JSON(fiber.Map{
			"data": config,
		})
	}
}
