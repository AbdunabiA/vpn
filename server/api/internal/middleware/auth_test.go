package middleware_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"vpnapp/server/api/internal/cache"
	"vpnapp/server/api/internal/middleware"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const testSecret = "test-jwt-secret-32-bytes-minimum!"

// buildToken creates a signed JWT with the given claims and TTL.
func buildToken(t *testing.T, userID, tier, role string, ttl time.Duration) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  userID,
		"tier": tier,
		"role": role,
		"iat":  now.Unix(),
		"exp":  now.Add(ttl).Unix(),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return tok
}

// tokenHash mirrors the hash computed inside AuthRequired.
func tokenHash(tokenString string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(tokenString)))
}

// newTestRedis starts a miniredis server and returns a connected client.
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

// statusOf sends a GET /protected request with the given Authorization header
// and returns the HTTP status code.
func statusOf(t *testing.T, app *fiber.App, authHeader string) int {
	t.Helper()
	req := httptest.NewRequest("GET", "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// protectedApp builds a minimal Fiber app with the AuthRequired middleware
// mounted on GET /protected.
func protectedApp(redisClient *redis.Client) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, _ error) error {
			return c.Status(fiber.StatusInternalServerError).SendString("internal error")
		},
	})
	// Tests pass nil for db to keep the existing behaviour (no user-
	// exists check). The new user-exists branch is exercised
	// separately once its own test helper for a test DB exists.
	app.Get("/protected",
		middleware.AuthRequired(testSecret, redisClient, nil),
		func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"user_id": c.Locals("user_id"),
				"tier":    c.Locals("tier"),
				"role":    c.Locals("role"),
			})
		},
	)
	return app
}

// --- Missing / malformed header ---

func TestAuthRequired_MissingHeader_Returns401(t *testing.T) {
	app := protectedApp(nil)
	if got := statusOf(t, app, ""); got != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", got)
	}
}

func TestAuthRequired_NoBearerPrefix_Returns401(t *testing.T) {
	app := protectedApp(nil)
	if got := statusOf(t, app, "token-without-bearer-prefix"); got != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", got)
	}
}

// --- Token validation ---

func TestAuthRequired_InvalidJWT_Returns401(t *testing.T) {
	app := protectedApp(nil)
	if got := statusOf(t, app, "Bearer not.a.valid.jwt"); got != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", got)
	}
}

func TestAuthRequired_ExpiredToken_Returns401(t *testing.T) {
	app := protectedApp(nil)
	tok := buildToken(t, "user-1", "free", "", -1*time.Minute)
	if got := statusOf(t, app, "Bearer "+tok); got != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", got)
	}
}

func TestAuthRequired_WrongSecret_Returns401(t *testing.T) {
	app := protectedApp(nil)
	// Sign with a different secret.
	now := time.Now()
	claims := jwt.MapClaims{"sub": "x", "exp": now.Add(time.Hour).Unix()}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("wrong-secret"))
	if got := statusOf(t, app, "Bearer "+tok); got != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", got)
	}
}

// --- Valid token, no Redis ---

func TestAuthRequired_ValidToken_NoRedis_Returns200(t *testing.T) {
	app := protectedApp(nil)
	tok := buildToken(t, "user-42", "premium", "admin", 15*time.Minute)
	if got := statusOf(t, app, "Bearer "+tok); got != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", got)
	}
}

// --- Valid token, with Redis, not blacklisted ---

func TestAuthRequired_ValidToken_WithRedis_NotBlacklisted_Returns200(t *testing.T) {
	redisClient, _ := newTestRedis(t)
	app := protectedApp(redisClient)
	tok := buildToken(t, "user-10", "free", "", 15*time.Minute)
	if got := statusOf(t, app, "Bearer "+tok); got != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", got)
	}
}

// --- Blacklisted token ---

func TestAuthRequired_BlacklistedToken_Returns401(t *testing.T) {
	redisClient, _ := newTestRedis(t)
	app := protectedApp(redisClient)

	tok := buildToken(t, "user-blacklisted", "premium", "", 15*time.Minute)

	// Blacklist the token through the cache package — the same path a logout
	// handler would take.
	if err := cache.BlacklistToken(context.Background(), redisClient, tokenHash(tok), 15*time.Minute); err != nil {
		t.Fatalf("BlacklistToken error: %v", err)
	}

	if got := statusOf(t, app, "Bearer "+tok); got != fiber.StatusUnauthorized {
		t.Fatalf("expected 401 for blacklisted token, got %d", got)
	}
}

// --- Context locals are populated correctly ---

func TestAuthRequired_LocalsPopulated(t *testing.T) {
	var capturedID, capturedTier, capturedRole string

	app := fiber.New()
	app.Get("/protected",
		middleware.AuthRequired(testSecret, nil, nil),
		func(c *fiber.Ctx) error {
			capturedID, _ = c.Locals("user_id").(string)
			capturedTier, _ = c.Locals("tier").(string)
			capturedRole, _ = c.Locals("role").(string)
			return c.SendStatus(fiber.StatusOK)
		},
	)

	tok := buildToken(t, "uid-123", "ultimate", "superadmin", 15*time.Minute)
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if capturedID != "uid-123" {
		t.Errorf("user_id: expected uid-123, got %q", capturedID)
	}
	if capturedTier != "ultimate" {
		t.Errorf("tier: expected ultimate, got %q", capturedTier)
	}
	if capturedRole != "superadmin" {
		t.Errorf("role: expected superadmin, got %q", capturedRole)
	}
}
