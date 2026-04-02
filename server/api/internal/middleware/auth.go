package middleware

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"vpnapp/server/api/internal/cache"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// Claims holds the JWT payload.
type Claims struct {
	UserID string `json:"sub"`
	Tier   string `json:"tier"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// AuthRequired is middleware that validates JWT access tokens.
// Extracts the user ID, subscription tier, and role from the token
// and stores them in the Fiber context locals.
//
// When a non-nil Redis client is provided, every request additionally checks
// whether the token has been blacklisted (e.g. after logout). A blacklisted
// token is rejected with 401 even if its signature and expiry are still valid.
func AuthRequired(jwtSecret string, redisClient *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract token from Authorization header.
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authorization header",
			})
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization format",
			})
		}

		// Parse and validate JWT.
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		// Check token blacklist when Redis is available.
		if redisClient != nil {
			tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenString)))
			if cache.IsTokenBlacklisted(c.Context(), redisClient, tokenHash) {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "token has been revoked",
				})
			}
		}

		// Store user info in context for downstream handlers.
		c.Locals("user_id", claims.UserID)
		c.Locals("tier", claims.Tier)
		c.Locals("role", claims.Role)

		return c.Next()
	}
}
