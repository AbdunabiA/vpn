package handler

import (
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"vpnapp/server/api/internal/config"
	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// linkCodeTTL controls how long a generated share code is valid for.
// 5 minutes is short enough that brute-forcing 10^6 codes over the rate
// limiter is impractical, but long enough to read it aloud over a phone call.
const linkCodeTTL = 5 * time.Minute

// generateLinkCode returns a 6-digit zero-padded numeric code.
// Uses crypto/rand so codes are unguessable.
func generateLinkCode() (string, error) {
	max := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	// %06d zero-pads so codes like 000_042 are still 6 digits.
	return padCode(n.Int64()), nil
}

func padCode(n int64) string {
	s := []byte{'0', '0', '0', '0', '0', '0'}
	for i := 5; i >= 0 && n > 0; i-- {
		s[i] = byte('0' + n%10)
		n /= 10
	}
	return string(s)
}

// CreateShareCode handles POST /devices/share-code.
// Generates a one-time, short-lived 6-digit code that a friend's device can
// redeem via /auth/link to attach itself to the caller's account.
//
// Refuses if the caller already has an unexpired code outstanding (one
// in-flight share at a time per user) or if their device cap is already at
// the maximum (no point sharing if there is no slot to fill).
func CreateShareCode(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)

		// Refuse if a code is already outstanding for this user.
		active, err := repository.CountActiveLinkCodesForUser(db, userID)
		if err != nil {
			logger.Error("share-code: count active failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}
		if active > 0 {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "a share code is already active for this account",
			})
		}

		// Refuse if the user's device cap leaves no room for an additional device.
		user, err := repository.FindUserByID(db, userID)
		if err != nil {
			logger.Error("share-code: load user failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}
		tier := user.SubscriptionTier
		if tier == "" {
			tier = "free"
		}
		limits, ok := model.PlanLimits[tier]
		if !ok {
			limits = model.PlanLimits["free"]
		}
		if limits.MaxDevices != model.UnlimitedDevices {
			deviceCount, err := repository.CountDevicesByUser(db, userID)
			if err != nil {
				logger.Error("share-code: count devices failed", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "internal server error",
				})
			}
			if deviceCount >= int64(limits.MaxDevices) {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error":       "device limit already reached on this plan",
					"max_devices": limits.MaxDevices,
				})
			}
		}

		// Generate a unique code, retrying on the rare collision (1 in 10^6).
		var code string
		for attempt := 0; attempt < 5; attempt++ {
			candidate, err := generateLinkCode()
			if err != nil {
				logger.Error("share-code: rng failed", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "internal server error",
				})
			}
			lc := &model.LinkCode{
				Code:      candidate,
				UserID:    userID,
				ExpiresAt: time.Now().Add(linkCodeTTL),
			}
			if err := repository.CreateLinkCode(db, lc); err != nil {
				if errors.Is(err, repository.ErrDuplicate) {
					continue // try again with a different number
				}
				logger.Error("share-code: insert failed", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "internal server error",
				})
			}
			code = candidate
			break
		}
		if code == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "could not allocate a unique code",
			})
		}

		logger.Info("share-code created",
			zap.String("user_id", userID),
			zap.Int("expires_in_sec", int(linkCodeTTL.Seconds())),
		)

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"data": fiber.Map{
				"code":           code,
				"expires_in_sec": int(linkCodeTTL.Seconds()),
			},
		})
	}
}

type linkRequest struct {
	Code     string `json:"code"`
	DeviceID string `json:"device_id"`
	Platform string `json:"platform"`
	Model    string `json:"model"`
}

// LinkDevice handles POST /auth/link.
//
// The caller is a brand-new device that holds a 6-digit code given out by an
// existing plan owner. We:
//   1. Atomically consume the code (one-time use).
//   2. Reassign the caller's device row to the owner's user_id (or create it).
//   3. Issue fresh JWT tokens for the owner so the caller's app starts behaving
//      as if it had logged into the owner's account.
//
// This endpoint is intentionally NOT auth-protected — the caller is exactly
// the unauthenticated guest device that wants to attach to a paid account.
// The version-gate middleware still requires X-App-Version, so old APKs
// cannot reach this endpoint.
func LinkDevice(logger *zap.Logger, cfg *config.Config, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req linkRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid request body",
			})
		}
		if req.Code == "" || req.DeviceID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "code and device_id are required",
			})
		}

		// Atomically claim the code so concurrent redemptions can only succeed once.
		lc, err := repository.ConsumeLinkCode(db, req.Code)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "invalid or expired code",
				})
			}
			logger.Error("link: consume failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Refuse if the owner's device cap is already full. Counting now and
		// not at code creation prevents the case where a device leaves the
		// plan between issuing and redeeming and somebody else claims the slot.
		owner, err := repository.FindUserByID(db, lc.UserID)
		if err != nil {
			logger.Error("link: owner not found", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}
		tier := owner.SubscriptionTier
		if tier == "" {
			tier = "free"
		}
		limits, ok := model.PlanLimits[tier]
		if !ok {
			limits = model.PlanLimits["free"]
		}
		if limits.MaxDevices != model.UnlimitedDevices {
			// Count devices currently bound to the owner. If the redeeming
			// device is already bound to the owner (link replay), we should
			// not double-count it — but ReassignDeviceUser is idempotent so
			// this is just a soft check that prevents over-allocation.
			count, err := repository.CountDevicesByUser(db, owner.ID)
			if err != nil {
				logger.Error("link: count devices failed", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "internal server error",
				})
			}
			// Check whether the redeeming device is already bound to the owner;
			// if so, the count already includes it and no new slot is needed.
			alreadyBoundToOwner := false
			if existing, err := repository.FindDeviceByDeviceID(db, req.DeviceID); err == nil {
				if existing.UserID == owner.ID {
					alreadyBoundToOwner = true
				}
			}
			if !alreadyBoundToOwner && count >= int64(limits.MaxDevices) {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error":       "owner's device limit reached",
					"max_devices": limits.MaxDevices,
				})
			}
		}

		// Reassign or create the device row pointing at the owner.
		if err := repository.ReassignDeviceUser(db, req.DeviceID, owner.ID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				device := model.Device{
					UserID:   owner.ID,
					DeviceID: req.DeviceID,
					Platform: req.Platform,
					Model:    req.Model,
				}
				if err := repository.CreateDevice(db, &device); err != nil {
					logger.Error("link: create device failed", zap.Error(err))
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
						"error": "internal server error",
					})
				}
			} else {
				logger.Error("link: reassign device failed", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "internal server error",
				})
			}
		}

		// Issue tokens that authenticate the caller as the owner.
		tokens, err := generateTokens(owner.ID, owner.SubscriptionTier, owner.Role, owner.FullName, cfg.JWTSecret)
		if err != nil {
			logger.Error("link: token generation failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}
		_ = storeRefreshSession(db, owner.ID, tokens.RefreshToken)

		logger.Info("device linked to owner",
			zap.String("owner_user_id", owner.ID),
			zap.String("device_id", req.DeviceID),
		)

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"data": tokens,
		})
	}
}

// ListMyDevices handles GET /devices.
// Returns the list of devices currently bound to the calling user.
func ListMyDevices(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)
		devices, err := repository.ListDevicesByUser(db, userID)
		if err != nil {
			logger.Error("list-devices failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}
		return c.JSON(fiber.Map{"data": devices})
	}
}

// DeleteMyDevice handles DELETE /devices/:id.
//
// Removes one device row from the calling user's quota. Used by the plan
// owner to evict a slot occupied by a device that has gone away (lost
// phone, factory reset, iOS reinstall that generated a new IDFV, friend
// who never came back).
//
// Authorisation: a user can only delete devices they currently own.
// The repository check enforces this — passing someone else's device id
// returns 404 just like a non-existent id.
func DeleteMyDevice(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)
		deviceRowID := c.Params("id")
		if deviceRowID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "device id required",
			})
		}

		if err := repository.DeleteDeviceByOwner(db, deviceRowID, userID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "device not found",
				})
			}
			logger.Error("delete-device failed",
				zap.String("user_id", userID),
				zap.String("device_row_id", deviceRowID),
				zap.Error(err),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		logger.Info("device removed",
			zap.String("user_id", userID),
			zap.String("device_row_id", deviceRowID),
		)
		return c.SendStatus(fiber.StatusNoContent)
	}
}
