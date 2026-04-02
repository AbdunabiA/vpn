package handler

import (
	"errors"
	"strconv"

	"vpnapp/server/api/internal/repository"

	"vpnapp/server/api/internal/model"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// parsePagination extracts page and limit from query params with safe defaults.
// page defaults to 1; limit defaults to 20 and is capped at 100.
func parsePagination(c *fiber.Ctx) (page, limit int) {
	page, _ = strconv.Atoi(c.Query("page", "1"))
	limit, _ = strconv.Atoi(c.Query("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

// AdminListUsers handles GET /admin/users.
// Query params: page (default 1), limit (default 20, max 100), search (partial email_hash match).
func AdminListUsers(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, limit := parsePagination(c)
		search := c.Query("search")

		users, total, err := repository.ListUsers(db, page, limit, search)
		if err != nil {
			logger.Error("admin: failed to list users", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Info("admin: listed users",
			zap.Int("page", page),
			zap.Int("limit", limit),
			zap.Int64("total", total),
		)

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"users": users,
				"pagination": fiber.Map{
					"page":        page,
					"limit":       limit,
					"total":       total,
					"total_pages": (total + int64(limit) - 1) / int64(limit),
				},
			},
		})
	}
}

// AdminGetUser handles GET /admin/users/:id.
// Returns the full user record for the given UUID.
func AdminGetUser(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Params("id")
		if userID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "user id required",
			})
		}

		user, err := repository.FindUserByIDAdmin(db, userID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "user not found",
				})
			}
			logger.Error("admin: failed to get user", zap.String("user_id", userID), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		return c.JSON(fiber.Map{
			"data": user,
		})
	}
}

// adminUpdateUserRequest defines the fields an admin may change on a user.
type adminUpdateUserRequest struct {
	SubscriptionTier string `json:"subscription_tier"`
	Role             string `json:"role"`
}

// AdminUpdateUser handles PATCH /admin/users/:id.
// Accepts subscription_tier and/or role; only provided fields are updated.
func AdminUpdateUser(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Params("id")
		if userID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "user id required",
			})
		}

		var req adminUpdateUserRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid request body",
			})
		}

		updates := make(map[string]interface{})
		if req.SubscriptionTier != "" {
			if _, ok := model.PlanLimits[req.SubscriptionTier]; !ok {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "subscription_tier must be one of: free, premium, ultimate",
				})
			}
			updates["subscription_tier"] = req.SubscriptionTier
		}
		if req.Role != "" {
			if req.Role != "user" && req.Role != "admin" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "role must be 'user' or 'admin'",
				})
			}
			updates["role"] = req.Role
		}

		if len(updates) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "no updatable fields provided",
			})
		}

		if err := repository.UpdateUser(db, userID, updates); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "user not found",
				})
			}
			logger.Error("admin: failed to update user", zap.String("user_id", userID), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Info("admin: updated user", zap.String("user_id", userID), zap.Any("updates", updates))

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"id":      userID,
				"updated": updates,
			},
		})
	}
}

// AdminListServers handles GET /admin/servers.
// Returns all VPN servers including inactive ones.
func AdminListServers(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		servers, err := repository.ListAllServers(db)
		if err != nil {
			logger.Error("admin: failed to list servers", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Debug("admin: listed servers", zap.Int("count", len(servers)))

		return c.JSON(fiber.Map{
			"data": servers,
		})
	}
}

// adminCreateServerRequest holds all required and optional fields for a new server.
type adminCreateServerRequest struct {
	Hostname         string `json:"hostname"`
	IPAddress        string `json:"ip_address"`
	Region           string `json:"region"`
	City             string `json:"city"`
	Country          string `json:"country"`
	CountryCode      string `json:"country_code"`
	Protocol         string `json:"protocol"`
	Capacity         int    `json:"capacity"`
	RealityPublicKey string `json:"reality_public_key"`
	RealityShortID   string `json:"reality_short_id"`
}

// AdminCreateServer handles POST /admin/servers.
// Creates a new VPN server with all fields; capacity defaults to 500 if omitted.
func AdminCreateServer(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req adminCreateServerRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid request body",
			})
		}

		if req.Hostname == "" || req.IPAddress == "" || req.Region == "" ||
			req.City == "" || req.Country == "" || req.CountryCode == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "hostname, ip_address, region, city, country, and country_code are required",
			})
		}

		if len(req.CountryCode) != 2 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "country_code must be a 2-character ISO code",
			})
		}

		protocol := req.Protocol
		if protocol == "" {
			protocol = "vless-reality"
		}
		capacity := req.Capacity
		if capacity <= 0 {
			capacity = 500
		}

		server := model.VPNServer{
			Hostname:         req.Hostname,
			IPAddress:        req.IPAddress,
			Region:           req.Region,
			City:             req.City,
			Country:          req.Country,
			CountryCode:      req.CountryCode,
			Protocol:         protocol,
			Capacity:         capacity,
			IsActive:         true,
			RealityPublicKey: req.RealityPublicKey,
			RealityShortID:   req.RealityShortID,
		}

		if err := repository.CreateServer(db, &server); err != nil {
			if errors.Is(err, repository.ErrDuplicate) {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": "hostname already exists",
				})
			}
			logger.Error("admin: failed to create server", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Info("admin: created server",
			zap.String("server_id", server.ID),
			zap.String("hostname", server.Hostname),
		)

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"data": server,
		})
	}
}

// adminUpdateServerRequest defines fields an admin may update on a server.
type adminUpdateServerRequest struct {
	IPAddress        string `json:"ip_address"`
	Capacity         int    `json:"capacity"`
	IsActive         *bool  `json:"is_active"`
	Protocol         string `json:"protocol"`
	RealityPublicKey string `json:"reality_public_key"`
	RealityShortID   string `json:"reality_short_id"`
	CurrentLoad      *int   `json:"current_load"`
}

// AdminUpdateServer handles PATCH /admin/servers/:id.
// Only explicitly-provided fields are updated. Supports toggling is_active.
func AdminUpdateServer(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		if serverID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "server id required",
			})
		}

		var req adminUpdateServerRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid request body",
			})
		}

		updates := make(map[string]interface{})
		if req.IPAddress != "" {
			updates["ip_address"] = req.IPAddress
		}
		if req.Capacity > 0 {
			updates["capacity"] = req.Capacity
		}
		if req.IsActive != nil {
			updates["is_active"] = *req.IsActive
		}
		if req.Protocol != "" {
			updates["protocol"] = req.Protocol
		}
		if req.RealityPublicKey != "" {
			updates["reality_public_key"] = req.RealityPublicKey
		}
		if req.RealityShortID != "" {
			updates["reality_short_id"] = req.RealityShortID
		}
		if req.CurrentLoad != nil {
			updates["current_load"] = *req.CurrentLoad
		}

		if len(updates) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "no updatable fields provided",
			})
		}

		if err := repository.UpdateServer(db, serverID, updates); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "server not found",
				})
			}
			logger.Error("admin: failed to update server", zap.String("server_id", serverID), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Info("admin: updated server", zap.String("server_id", serverID), zap.Any("updates", updates))

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"id":      serverID,
				"updated": updates,
			},
		})
	}
}

// AdminDeleteServer handles DELETE /admin/servers/:id.
// Performs a soft delete by setting is_active = false.
func AdminDeleteServer(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		if serverID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "server id required",
			})
		}

		if err := repository.DeleteServer(db, serverID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "server not found",
				})
			}
			logger.Error("admin: failed to delete server", zap.String("server_id", serverID), zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Info("admin: soft-deleted server", zap.String("server_id", serverID))

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"id":      serverID,
				"deleted": true,
			},
		})
	}
}

// AdminGetStats handles GET /admin/stats.
// Returns aggregate dashboard numbers: user counts, subscription counts, server counts.
func AdminGetStats(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		stats, err := repository.GetGlobalStats(db)
		if err != nil {
			logger.Error("admin: failed to get stats", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		return c.JSON(fiber.Map{
			"data": stats,
		})
	}
}
