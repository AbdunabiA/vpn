package middleware_test

import (
	"io"
	"net/http/httptest"
	"testing"

	"vpnapp/server/api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// newAppVersionApp builds a minimal Fiber app whose only route tree is
// guarded by the version-gate middleware with the given minimum version
// and skip rules. The dummy handler always returns 200 OK so any non-200
// status observed by a test is necessarily produced by the middleware.
func newAppVersionApp(t *testing.T, minVersion string, rules ...middleware.SkipRule) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(middleware.AppVersion(minVersion, zap.NewNop(), rules...))
	app.All("/*", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

// doRequest drives a single request through the app and returns the status
// code plus the response body (drained for completeness — tests do not
// currently assert on body content but having it available makes failure
// diagnosis cheaper).
func doRequest(t *testing.T, app *fiber.App, method, path, versionHeader string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if versionHeader != "" {
		req.Header.Set("X-App-Version", versionHeader)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test(%s %s) failed: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestAppVersion_RejectsMissingHeader(t *testing.T) {
	app := newAppVersionApp(t, "2.0.0")
	status, _ := doRequest(t, app, fiber.MethodGet, "/api/v1/servers", "")
	if status != fiber.StatusUpgradeRequired {
		t.Fatalf("missing X-App-Version: expected 426, got %d", status)
	}
}

func TestAppVersion_RejectsOutdatedClient(t *testing.T) {
	app := newAppVersionApp(t, "2.0.0")
	status, _ := doRequest(t, app, fiber.MethodGet, "/api/v1/servers", "1.9.9")
	if status != fiber.StatusUpgradeRequired {
		t.Fatalf("old X-App-Version: expected 426, got %d", status)
	}
}

func TestAppVersion_AllowsCurrentClient(t *testing.T) {
	app := newAppVersionApp(t, "2.0.0")
	status, _ := doRequest(t, app, fiber.MethodGet, "/api/v1/servers", "2.0.0")
	if status != fiber.StatusOK {
		t.Fatalf("current X-App-Version: expected 200, got %d", status)
	}
}

func TestAppVersion_ExactSkipHonoursMethod(t *testing.T) {
	app := newAppVersionApp(
		t,
		"2.0.0",
		middleware.SkipRule{Method: fiber.MethodGet, Path: "/api/v1/health"},
	)

	// GET /health is skipped — no header required.
	if status, _ := doRequest(t, app, fiber.MethodGet, "/api/v1/health", ""); status != fiber.StatusOK {
		t.Fatalf("GET /health without header: expected 200, got %d", status)
	}
	// POST /health is NOT skipped — gate applies.
	if status, _ := doRequest(t, app, fiber.MethodPost, "/api/v1/health", ""); status != fiber.StatusUpgradeRequired {
		t.Fatalf("POST /health without header: expected 426, got %d", status)
	}
}

// TestAppVersion_RefreshSkipped asserts the web admin panel can hit
// POST /api/v1/auth/refresh without X-App-Version so the silent refresh
// flow doesn't log admins out every 5 minutes.
func TestAppVersion_RefreshSkipped(t *testing.T) {
	app := newAppVersionApp(
		t,
		"2.0.0",
		middleware.SkipRule{Method: fiber.MethodPost, Path: "/api/v1/auth/refresh"},
	)
	if status, _ := doRequest(t, app, fiber.MethodPost, "/api/v1/auth/refresh", ""); status != fiber.StatusOK {
		t.Fatalf("POST /auth/refresh without header: expected 200, got %d", status)
	}
}

// TestAppVersion_AdminPrefixSkipped asserts the entire /api/v1/admin/ route
// tree bypasses the mobile version gate (any method) so the web admin panel
// can call it without sending a fake X-App-Version. This is the single
// piece of middleware change that unblocks Phase B of the admin panel.
func TestAppVersion_AdminPrefixSkipped(t *testing.T) {
	app := newAppVersionApp(
		t,
		"2.0.0",
		middleware.SkipRule{Path: "/api/v1/admin/", Prefix: true},
	)

	cases := []struct {
		method, path string
	}{
		{fiber.MethodGet, "/api/v1/admin/users"},
		{fiber.MethodGet, "/api/v1/admin/users/da29ed49-8790-4ca7-95f0-b2d308108719"},
		{fiber.MethodPatch, "/api/v1/admin/users/da29ed49-8790-4ca7-95f0-b2d308108719"},
		{fiber.MethodDelete, "/api/v1/admin/servers/1"},
		{fiber.MethodGet, "/api/v1/admin/stats"},
	}
	for _, tc := range cases {
		status, _ := doRequest(t, app, tc.method, tc.path, "")
		if status != fiber.StatusOK {
			t.Errorf("%s %s without X-App-Version: expected 200, got %d", tc.method, tc.path, status)
		}
	}
}

// TestAppVersion_AdminPrefixDoesNotLeakToSiblings guards against the
// "/admin/" prefix accidentally matching sibling paths like "/adminx" or
// top-level "/admin" that are not intended to be exempt. The trailing
// slash on the prefix rule is load-bearing — this test would fail if the
// middleware stripped it, so it doubles as a regression alarm.
func TestAppVersion_AdminPrefixDoesNotLeakToSiblings(t *testing.T) {
	app := newAppVersionApp(
		t,
		"2.0.0",
		middleware.SkipRule{Path: "/api/v1/admin/", Prefix: true},
	)

	leaky := []string{
		"/api/v1/adminx/oops",
		"/api/v1/admin",          // no trailing slash — would incorrectly match
		"/api/v1/not-admin/back", // unrelated
	}
	for _, p := range leaky {
		status, _ := doRequest(t, app, fiber.MethodGet, p, "")
		if status != fiber.StatusUpgradeRequired {
			t.Errorf("prefix leak: GET %s without header: expected 426, got %d", p, status)
		}
	}
}

// TestAppVersion_PrefixRuleDefaultsToAnyMethod confirms that omitting
// Method on a Prefix rule matches every HTTP verb — the cleaner
// alternative to registering one rule per verb for the admin tree.
func TestAppVersion_PrefixRuleDefaultsToAnyMethod(t *testing.T) {
	app := newAppVersionApp(
		t,
		"2.0.0",
		middleware.SkipRule{Path: "/api/v1/admin/", Prefix: true}, // no Method
	)
	methods := []string{
		fiber.MethodGet, fiber.MethodPost, fiber.MethodPut,
		fiber.MethodPatch, fiber.MethodDelete,
	}
	for _, m := range methods {
		if status, _ := doRequest(t, app, m, "/api/v1/admin/users", ""); status != fiber.StatusOK {
			t.Errorf("%s /admin/users without header: expected 200, got %d", m, status)
		}
	}
}

// TestAppVersion_PrefixRuleHonoursExplicitMethod confirms that setting a
// specific Method on a Prefix rule narrows the bypass — other methods on
// the same prefix are still gated.
func TestAppVersion_PrefixRuleHonoursExplicitMethod(t *testing.T) {
	app := newAppVersionApp(
		t,
		"2.0.0",
		middleware.SkipRule{Method: fiber.MethodGet, Path: "/api/v1/admin/", Prefix: true},
	)

	if status, _ := doRequest(t, app, fiber.MethodGet, "/api/v1/admin/users", ""); status != fiber.StatusOK {
		t.Fatalf("GET /admin/users skipped: expected 200, got %d", status)
	}
	if status, _ := doRequest(t, app, fiber.MethodPost, "/api/v1/admin/users", ""); status != fiber.StatusUpgradeRequired {
		t.Fatalf("POST /admin/users gated: expected 426, got %d", status)
	}
}
