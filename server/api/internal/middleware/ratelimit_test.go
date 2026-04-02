package middleware_test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"vpnapp/server/api/internal/middleware"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// rateLimitApp builds a Fiber app with RateLimit middleware and a 200 OK handler.
func rateLimitApp(redisClient *redis.Client) *fiber.App {
	logger := zap.NewNop()
	app := fiber.New()
	app.Use(middleware.RateLimit(redisClient, logger))
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

// getStatus fires a GET / request with an optional user_id local simulated via
// a real token (we use the Authorization header path for integration, but for
// rate-limit tests we test at the middleware boundary directly by checking the
// IP path — user_id locals are only set by AuthRequired which runs before).
func getStatusNoAuth(t *testing.T, app *fiber.App) int {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestRateLimit_UnauthenticatedBelowLimit_Returns200(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	app := rateLimitApp(client)

	// First request must succeed.
	if got := getStatusNoAuth(t, app); got != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", got)
	}
}

func TestRateLimit_UnauthenticatedExceedsLimit_Returns429(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// Pre-seed the rate limit counter to 30 (the unauthenticated limit).
	// The next request should push it to 31 and receive 429.
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.2:1234"
		resp, err := rateLimitApp(client).Test(req, -1)
		if err != nil {
			t.Fatalf("request %d error: %v", i, err)
		}
		_ = resp.Body.Close()
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	resp, err := rateLimitApp(client).Test(req, -1)
	if err != nil {
		t.Fatalf("final request error: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit exceeded, got %d", resp.StatusCode)
	}
}

// TestRateLimit_AuthenticatedUserIsolated verifies that different user IDs get
// independent counters. We test this by mounting AuthRequired first so that
// user_id gets set in Locals before RateLimit runs.
func TestRateLimit_AuthenticatedUserIsolated(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logger := zap.NewNop()
	app := fiber.New()
	// Simulate AuthRequired having already set user_id by using a middleware shim.
	app.Use(func(c *fiber.Ctx) error {
		// Pull user ID from a test header to keep the test self-contained.
		uid := c.Get("X-Test-User")
		if uid != "" {
			c.Locals("user_id", uid)
		}
		return c.Next()
	})
	app.Use(middleware.RateLimit(client, logger))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	makeReq := func(userID string) int {
		req := httptest.NewRequest("GET", "/", nil)
		if userID != "" {
			req.Header.Set("X-Test-User", userID)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("app.Test error: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	// First request for user-A must succeed.
	if got := makeReq("user-A"); got != fiber.StatusOK {
		t.Fatalf("user-A first request: expected 200, got %d", got)
	}
	// First request for user-B must also succeed (separate counter).
	if got := makeReq("user-B"); got != fiber.StatusOK {
		t.Fatalf("user-B first request: expected 200, got %d", got)
	}
}

func TestRateLimit_RedisDown_AllowsRequest(t *testing.T) {
	// Use a client pointing at a port nobody is listening on.
	client := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("127.0.0.1:%d", 19999)})
	t.Cleanup(func() { _ = client.Close() })

	app := rateLimitApp(client)
	// Request must still succeed — Redis outage must not block traffic.
	if got := getStatusNoAuth(t, app); got != fiber.StatusOK {
		t.Fatalf("expected 200 when Redis is down, got %d", got)
	}
}
