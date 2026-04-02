package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"vpnapp/server/api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

// newTestApp builds a minimal Fiber app that sets a given role in locals and
// then runs AdminRequired, returning 200 OK on success.
func newTestApp(role string) *fiber.App {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals("role", role)
		return c.Next()
	}, middleware.AdminRequired(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestAdminRequired_AllowsAdminRole(t *testing.T) {
	app := newTestApp("admin")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAdminRequired_RejectsUserRole(t *testing.T) {
	app := newTestApp("user")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAdminRequired_RejectsEmptyRole(t *testing.T) {
	app := newTestApp("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAdminRequired_RejectsArbitraryRole(t *testing.T) {
	app := newTestApp("moderator")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAdminRequired_ResponseBodyContainsErrorKey(t *testing.T) {
	app := newTestApp("user")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if bodyStr == "" {
		t.Error("expected non-empty response body")
	}
	// The response must contain "forbidden" to match the handler convention.
	const want = "forbidden"
	found := false
	for i := 0; i <= len(bodyStr)-len(want); i++ {
		if bodyStr[i:i+len(want)] == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("response body %q does not contain %q", bodyStr, want)
	}
}
