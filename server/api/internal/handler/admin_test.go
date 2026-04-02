package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vpnapp/server/api/internal/handler"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// stubDB returns a *gorm.DB that is intentionally nil-valued so tests that
// do not exercise DB calls can still build a handler. Tests that need DB
// behaviour use table-driven stubs via the repository layer instead.
func stubDB() *gorm.DB { return nil }

func stubLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

// appWith wraps a single handler behind an auth-local injector so we can test
// handlers without a real JWT stack.
func appWith(role string, h fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		// Suppress the default 500 page so we can inspect JSON bodies.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", "test-user")
		c.Locals("role", role)
		return c.Next()
	})
	app.Use(h)
	return app
}

// --- AdminGetStats ---

func TestAdminGetStats_NilDB_Returns500(t *testing.T) {
	app := appWith("admin", handler.AdminGetStats(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// --- AdminListServers ---

func TestAdminListServers_NilDB_Returns500(t *testing.T) {
	app := appWith("admin", handler.AdminListServers(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// --- AdminListUsers ---

func TestAdminListUsers_NilDB_Returns500(t *testing.T) {
	app := appWith("admin", handler.AdminListUsers(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodGet, "/?page=1&limit=10", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// --- AdminGetUser ---

func TestAdminGetUser_MissingID_Returns400(t *testing.T) {
	// Build app with a route that has no :id param — the handler must return 400.
	app := fiber.New()
	app.Get("/", handler.AdminGetUser(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- AdminUpdateUser ---

func TestAdminUpdateUser_InvalidBody_Returns400(t *testing.T) {
	app := fiber.New()
	app.Patch("/:id", handler.AdminUpdateUser(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodPatch, "/some-uuid", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAdminUpdateUser_InvalidRole_Returns400(t *testing.T) {
	app := fiber.New()
	app.Patch("/:id", handler.AdminUpdateUser(stubLogger(), stubDB()))

	body, _ := json.Marshal(map[string]string{"role": "superadmin"})
	req := httptest.NewRequest(http.MethodPatch, "/some-uuid", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAdminUpdateUser_NoFields_Returns400(t *testing.T) {
	app := fiber.New()
	app.Patch("/:id", handler.AdminUpdateUser(stubLogger(), stubDB()))

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/some-uuid", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- AdminCreateServer ---

func TestAdminCreateServer_MissingRequiredFields_Returns400(t *testing.T) {
	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"empty body", map[string]interface{}{}},
		{"missing ip_address", map[string]interface{}{"hostname": "test", "region": "EU", "city": "X", "country": "Y", "country_code": "YY"}},
		{"bad country_code length", map[string]interface{}{"hostname": "h", "ip_address": "1.2.3.4", "region": "EU", "city": "X", "country": "Y", "country_code": "USA"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/", handler.AdminCreateServer(stubLogger(), stubDB()))

			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("unexpected request error: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("[%s] expected 400, got %d", tc.name, resp.StatusCode)
			}
		})
	}
}

// --- AdminUpdateServer ---

func TestAdminUpdateServer_MissingID_Returns400(t *testing.T) {
	app := fiber.New()
	app.Patch("/", handler.AdminUpdateServer(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAdminUpdateServer_NoFields_Returns400(t *testing.T) {
	app := fiber.New()
	app.Patch("/:id", handler.AdminUpdateServer(stubLogger(), stubDB()))

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPatch, "/some-uuid", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- AdminDeleteServer ---

func TestAdminDeleteServer_MissingID_Returns400(t *testing.T) {
	app := fiber.New()
	app.Delete("/", handler.AdminDeleteServer(stubLogger(), stubDB()))
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
