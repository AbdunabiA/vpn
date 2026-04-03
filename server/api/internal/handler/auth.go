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
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Register handles POST /auth/register.
// Creates a new user with hashed email + bcrypt password, returns JWT tokens.
func Register(logger *zap.Logger, cfg *config.Config, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req registerRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid request body",
			})
		}

		if req.Email == "" || len(req.Password) < 8 || len(req.Password) > 72 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "email required, password must be 8-72 characters",
			})
		}

		if len(req.Name) < 2 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "name must be at least 2 characters",
			})
		}

		if !strings.Contains(req.Email, "@") || len(req.Email) < 5 || len(req.Email) > 255 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid email format",
			})
		}

		// Hash email for zero-knowledge storage
		emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Email)))

		// Hash password with bcrypt
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			logger.Error("failed to hash password", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Create user in database
		user := model.User{
			EmailHash:    emailHash,
			PasswordHash: string(passwordHash),
			FullName:     req.Name,
		}
		if err := repository.CreateUser(db, &user); err != nil {
			if errors.Is(err, repository.ErrDuplicate) {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": "email already registered",
				})
			}
			logger.Error("failed to create user", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Create default free subscription.
		// If this fails we must roll back the user record so registration is
		// atomic — a user without a subscription would be unusable.
		sub := model.Subscription{
			UserID:   user.ID,
			Plan:     "free",
			IsActive: true,
		}
		if err := repository.CreateSubscription(db, &sub); err != nil {
			logger.Error("failed to create subscription — rolling back user creation",
				zap.String("user_id", user.ID),
				zap.Error(err),
			)
			if deleteErr := repository.DeleteUser(db, user.ID); deleteErr != nil {
				logger.Error("failed to roll back user after subscription creation failure",
					zap.String("user_id", user.ID),
					zap.Error(deleteErr),
				)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Generate JWT tokens — new users always start with role "user"
		tokens, err := generateTokens(user.ID, "free", "user", user.FullName, cfg.JWTSecret)
		if err != nil {
			logger.Error("failed to generate tokens", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Store refresh token session
		if err := storeRefreshSession(db, user.ID, tokens.RefreshToken); err != nil {
			logger.Error("failed to store session", zap.Error(err))
		}

		logger.Info("user registered", zap.String("user_id", user.ID))

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"data": tokens,
		})
	}
}

// Login handles POST /auth/login.
// Validates credentials against DB and returns JWT tokens.
func Login(logger *zap.Logger, cfg *config.Config, db *gorm.DB) fiber.Handler {
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
			logger.Error("failed to find user", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid credentials",
			})
		}

		// Generate tokens with real user ID, tier, and role
		tokens, err := generateTokens(user.ID, user.SubscriptionTier, user.Role, user.FullName, cfg.JWTSecret)
		if err != nil {
			logger.Error("failed to generate tokens", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Store refresh token session
		if err := storeRefreshSession(db, user.ID, tokens.RefreshToken); err != nil {
			logger.Error("failed to store session", zap.Error(err))
		}

		logger.Info("user logged in", zap.String("user_id", user.ID))

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
