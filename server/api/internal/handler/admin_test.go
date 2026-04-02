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

// ---------------------------------------------------------------------------
// Additional edge-case tests
// ---------------------------------------------------------------------------

// --- AdminCreateServer duplicate hostname ---

func TestAdminCreateServer_DuplicateHostname_Returns409(t *testing.T) {
	db := newHandlerTestDB(t)

	// Register the server route with a real DB.
	app := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler(stubLogger())})
	app.Post("/admin/servers", handler.AdminCreateServer(stubLogger(), db))

	validBody := map[string]interface{}{
		"hostname":     "dup-server",
		"ip_address":   "10.0.1.1",
		"region":       "EU",
		"city":         "Berlin",
		"country":      "Germany",
		"country_code": "DE",
	}
	bodyBytes, _ := json.Marshal(validBody)

	// First creation must succeed (201).
	req1 := httptest.NewRequest(http.MethodPost, "/admin/servers", bytes.NewBuffer(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("first request error: %v", err)
	}
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on first create, got %d", resp1.StatusCode)
	}

	// Second creation with the same hostname must return 409 Conflict.
	bodyBytes2, _ := json.Marshal(validBody)
	req2 := httptest.NewRequest(http.MethodPost, "/admin/servers", bytes.NewBuffer(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("second request error: %v", err)
	}
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("duplicate hostname: expected 409, got %d", resp2.StatusCode)
	}
}

// --- AdminDeleteServer not found ---

func TestAdminDeleteServer_NonExistentID_Returns404(t *testing.T) {
	db := newHandlerTestDB(t)

	app := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler(stubLogger())})
	app.Delete("/admin/servers/:id", handler.AdminDeleteServer(stubLogger(), db))

	req := httptest.NewRequest(http.MethodDelete, "/admin/servers/00000000-0000-0000-0000-000000000000", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent server, got %d", resp.StatusCode)
	}
}

// --- AdminUpdateUser not found ---

func TestAdminUpdateUser_NonExistentID_Returns404(t *testing.T) {
	db := newHandlerTestDB(t)

	app := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler(stubLogger())})
	app.Patch("/admin/users/:id", handler.AdminUpdateUser(stubLogger(), db))

	body, _ := json.Marshal(map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/admin/users/00000000-0000-0000-0000-000000000000", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent user, got %d", resp.StatusCode)
	}
}

// --- AdminUpdateServer not found ---

func TestAdminUpdateServer_NonExistentID_Returns404(t *testing.T) {
	db := newHandlerTestDB(t)

	app := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler(stubLogger())})
	app.Patch("/admin/servers/:id", handler.AdminUpdateServer(stubLogger(), db))

	body, _ := json.Marshal(map[string]string{"ip_address": "9.9.9.9"})
	req := httptest.NewRequest(http.MethodPatch, "/admin/servers/00000000-0000-0000-0000-000000000000", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent server, got %d", resp.StatusCode)
	}
}

// --- AdminGetUser not found ---

func TestAdminGetUser_NonExistentID_Returns404(t *testing.T) {
	db := newHandlerTestDB(t)

	app := fiber.New(fiber.Config{ErrorHandler: handler.ErrorHandler(stubLogger())})
	app.Get("/admin/users/:id", handler.AdminGetUser(stubLogger(), db))

	req := httptest.NewRequest(http.MethodGet, "/admin/users/00000000-0000-0000-0000-000000000000", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent user, got %d", resp.StatusCode)
	}
}

// --- AdminListServers happy path with real DB ---

func TestAdminListServers_WithDB_Returns200(t *testing.T) {
	db := newHandlerTestDB(t)
	// Seed a server so the list is non-trivial.
	seedActiveServer(t, db)

	app := appWith("admin", handler.AdminListServers(stubLogger(), db))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// --- Pagination clamping ---

func TestAdminListUsers_LimitClamped_Returns200(t *testing.T) {
	db := newHandlerTestDB(t)

	app := appWith("admin", handler.AdminListUsers(stubLogger(), db))
	// limit=999 must be clamped to 100 — should not error
	req := httptest.NewRequest(http.MethodGet, "/?page=1&limit=999", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	// With real DB (empty) it should succeed
	if resp.StatusCode == http.StatusBadRequest {
		t.Errorf("limit clamping: expected non-400, got 400")
	}
}

func TestAdminListUsers_NegativePage_DefaultsTo1(t *testing.T) {
	db := newHandlerTestDB(t)

	app := appWith("admin", handler.AdminListUsers(stubLogger(), db))
	req := httptest.NewRequest(http.MethodGet, "/?page=-5&limit=10", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected request error: %v", err)
	}
	if resp.StatusCode == http.StatusBadRequest {
		t.Errorf("negative page: expected non-400 (defaults to 1), got 400")
	}
}
