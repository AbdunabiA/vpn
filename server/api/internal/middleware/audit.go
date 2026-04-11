package middleware

import (
	"strings"

	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AuditLog wraps admin handlers so that any mutating request whose
// handler returns a 2xx status is persisted to the audit_log table.
// Must run AFTER AuthRequired and AdminRequired — it reads user_id
// from c.Locals.
//
// Why post-handler instead of pre-handler:
//   Running after c.Next() lets us skip audit writes for requests
//   that failed validation (400), auth (401), or not-found (404).
//   Otherwise every typo in a curl would spam the table.
//
// Only GET requests are free — writes (POST/PATCH/PUT/DELETE) are
// recorded. For write methods where the handler rejected the request
// with a non-2xx, we skip the write entirely.
func AuditLog(db *gorm.DB, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Not a mutation — nothing to audit.
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			return c.Next()
		}

		// Let the handler run first so we can short-circuit on failure.
		if err := c.Next(); err != nil {
			return err
		}
		if c.Response().StatusCode() >= 300 {
			return nil
		}

		// Best-effort extraction of the acting admin's UUID. If Locals
		// is missing (e.g. the middleware stack changed), skip the
		// audit write — log, don't fail the request.
		adminID, _ := c.Locals("user_id").(string)
		if adminID == "" {
			logger.Warn("audit: missing user_id in locals, skipping",
				zap.String("path", c.Path()),
			)
			return nil
		}

		action := describeAction(c.Method(), c.Path())
		target := extractTargetID(c.Path())

		var targetPtr *string
		if target != "" {
			t := target
			targetPtr = &t
		}

		// The request body is only reliably available post-handler
		// when BodyParser stored a copy; Fiber does not expose a
		// parsed map we can snapshot. Instead, we record the path
		// params and the query string — enough for a human to
		// reconstruct "what did this admin touch".
		details := model.AuditDetails{
			"method": c.Method(),
			"path":   c.Path(),
		}
		if q := c.Request().URI().QueryString(); len(q) > 0 {
			details["query"] = string(q)
		}
		for k, v := range c.AllParams() {
			details["param_"+k] = v
		}

		entry := &model.AuditLogEntry{
			AdminID:  adminID,
			Action:   action,
			TargetID: targetPtr,
			Details:  details,
			IP:       c.IP(),
		}
		if err := repository.CreateAuditEntry(db, entry); err != nil {
			// Never fail the admin action because audit persistence
			// failed — the mutation already succeeded. Log loudly.
			logger.Error("audit: failed to persist entry",
				zap.String("action", action),
				zap.String("admin_id", adminID),
				zap.Error(err),
			)
		}
		return nil
	}
}

// describeAction maps (method, path) to a short stable action name like
// "update_user" or "delete_server". The name is what the UI's audit
// log page displays in the Action column, so keep it readable.
func describeAction(method, path string) string {
	// Strip the /api/v1 prefix so the keys are independent of
	// versioning. Keep the verb short — "update", "delete",
	// "create", "logout", "password".
	stripped := strings.TrimPrefix(path, "/api/v1")

	switch {
	case method == fiber.MethodPost && strings.HasSuffix(stripped, "/auth/admin/change-password"):
		return "change_password"
	case method == fiber.MethodPost && strings.HasSuffix(stripped, "/auth/admin-login"):
		return "admin_login"
	case method == fiber.MethodPatch && strings.HasPrefix(stripped, "/admin/users/"):
		return "update_user"
	case method == fiber.MethodDelete && strings.Contains(stripped, "/devices/"):
		return "delete_device"
	case method == fiber.MethodDelete && strings.HasPrefix(stripped, "/admin/users/"):
		return "delete_user"
	case method == fiber.MethodDelete && strings.HasPrefix(stripped, "/admin/servers/"):
		return "delete_server"
	case method == fiber.MethodPatch && strings.HasPrefix(stripped, "/admin/servers/"):
		return "update_server"
	case method == fiber.MethodPost && strings.HasPrefix(stripped, "/admin/servers"):
		return "create_server"
	}
	return strings.ToLower(method) + "_" + stripped
}

// extractTargetID pulls the primary :id path parameter for the common
// URL shapes under /admin/. Returns an empty string when there is no
// obvious target (create, list, stats endpoints).
func extractTargetID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Look for the first UUID-shaped (36-char) segment after "admin".
	for i, p := range parts {
		if p == "admin" && i+2 < len(parts) {
			id := parts[i+2]
			if len(id) == 36 && strings.Count(id, "-") == 4 {
				return id
			}
		}
	}
	return ""
}
