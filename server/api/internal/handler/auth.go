package handler

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"vpnapp/server/api/internal/config"
	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// AdminLogin handles POST /auth/admin-login.
// Validates email+password against DB and returns JWT tokens ONLY if the user
// has role='admin'. Non-admin users receive the same "invalid credentials"
// error as a wrong password (no role-enumeration leak).
func AdminLogin(logger *zap.Logger, cfg *config.Config, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req loginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid request body",
			})
		}

		if req.Email == "" || req.Password == "" || len(req.Password) > 72 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "email and password required (max 72 chars)",
			})
		}

		if !strings.Contains(req.Email, "@") || len(req.Email) < 5 || len(req.Email) > 255 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid email format",
			})
		}

		// Find user by email hash
		emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Email)))
		user, err := repository.FindUserByEmailHash(db, emailHash)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "invalid credentials",
				})
			}
			logger.Error("admin-login: failed to find user", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Verify password
		if user.PasswordHash == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid credentials",
			})
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid credentials",
			})
		}

		// Enforce admin role — non-admins get the same error as wrong password
		if user.Role != "admin" {
			logger.Warn("admin-login: non-admin user attempted admin login", zap.String("user_id", user.ID))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid credentials",
			})
		}

		tokens, err := generateTokens(user.ID, user.SubscriptionTier, user.Role, user.FullName, cfg.JWTSecret)
		if err != nil {
			logger.Error("admin-login: failed to generate tokens", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		if err := storeRefreshSession(db, user.ID, tokens.RefreshToken); err != nil {
			logger.Error("admin-login: failed to store session", zap.Error(err))
		}

		logger.Info("admin logged in", zap.String("user_id", user.ID))

		return c.JSON(fiber.Map{
			"data": tokens,
		})
	}
}

// RefreshToken handles POST /auth/refresh.
// Validates refresh token, rotates tokens, returns new pair.
func RefreshToken(logger *zap.Logger, cfg *config.Config, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.BodyParser(&req); err != nil || req.RefreshToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "refresh_token required",
			})
		}

		// Find session by refresh token hash
		tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.RefreshToken)))
		session, err := repository.FindSessionByTokenHash(db, tokenHash)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "invalid or expired refresh token",
				})
			}
			logger.Error("failed to find session", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Delete old session (token rotation)
		repository.DeleteSession(db, session.ID)

		// Look up user for current tier
		user, err := repository.FindUserByID(db, session.UserID)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "user not found",
			})
		}

		// Generate new token pair with current role
		tokens, err := generateTokens(user.ID, user.SubscriptionTier, user.Role, user.FullName, cfg.JWTSecret)
		if err != nil {
			logger.Error("failed to generate tokens", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Store new refresh session
		if err := storeRefreshSession(db, user.ID, tokens.RefreshToken); err != nil {
			logger.Error("failed to store session", zap.Error(err))
		}

		return c.JSON(fiber.Map{
			"data": tokens,
		})
	}
}

// GuestLogin handles POST /auth/guest.
// Creates an anonymous account with a random name, no email or password, and
// returns JWT tokens so the app can operate without requiring registration.
func GuestLogin(logger *zap.Logger, db *gorm.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Generate a short random suffix from a UUID for the display name
		suffix := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
		guestName := "guest_" + suffix

		user := model.User{
			FullName: guestName,
			// EmailHash and PasswordHash left nil — guest account
		}
		if err := repository.CreateUser(db, &user); err != nil {
			logger.Error("failed to create guest user", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		sub := model.Subscription{
			UserID:   user.ID,
			Plan:     "free",
			IsActive: true,
		}
		if err := repository.CreateSubscription(db, &sub); err != nil {
			logger.Error("failed to create guest subscription — rolling back user",
				zap.String("user_id", user.ID),
				zap.Error(err),
			)
			if deleteErr := repository.DeleteUser(db, user.ID); deleteErr != nil {
				logger.Error("failed to roll back guest user after subscription failure",
					zap.String("user_id", user.ID),
					zap.Error(deleteErr),
				)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		tokens, err := generateTokens(user.ID, "free", "user", user.FullName, cfg.JWTSecret)
		if err != nil {
			logger.Error("failed to generate guest tokens", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		if err := storeRefreshSession(db, user.ID, tokens.RefreshToken); err != nil {
			logger.Error("failed to store guest session", zap.Error(err))
		}

		logger.Info("guest user created", zap.String("user_id", user.ID), zap.String("name", guestName))

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"data": tokens,
		})
	}
}

// storeRefreshSession hashes the refresh token and stores it in the sessions table.
func storeRefreshSession(db *gorm.DB, userID, refreshToken string) error {
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))
	session := model.Session{
		UserID:           userID,
		RefreshTokenHash: tokenHash,
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
	}
	return repository.CreateSession(db, &session)
}

// generateTokens creates a JWT access token (15 min) and refresh token (30 days).
// The role claim is embedded in the access token for admin middleware checks.
// The name claim carries the user's display name so the app can show it without
// a separate /account call immediately after login/register.
func generateTokens(userID, tier, role, name, secret string) (*authResponse, error) {
	now := time.Now()
	accessExpiry := now.Add(15 * time.Minute)

	accessClaims := jwt.MapClaims{
		"sub":  userID,
		"tier": tier,
		"role": role,
		"name": name,
		"iat":  now.Unix(),
		"exp":  accessExpiry.Unix(),
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessString, err := accessToken.SignedString([]byte(secret))
	if err != nil {
		return nil, fmt.Errorf("signing access token: %w", err)
	}

	refreshClaims := jwt.MapClaims{
		"sub":  userID,
		"type": "refresh",
		"iat":  now.Unix(),
		"exp":  now.Add(30 * 24 * time.Hour).Unix(),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshString, err := refreshToken.SignedString([]byte(secret))
	if err != nil {
		return nil, fmt.Errorf("signing refresh token: %w", err)
	}

	return &authResponse{
		AccessToken:  accessString,
		RefreshToken: refreshString,
		ExpiresIn:    int(time.Until(accessExpiry).Seconds()),
	}, nil
}
