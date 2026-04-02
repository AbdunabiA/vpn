package middleware

import "github.com/gofiber/fiber/v2"

// AdminRequired is middleware that enforces admin-only access.
// It must run after AuthRequired, which populates c.Locals("role").
// Returns 403 Forbidden if the authenticated user is not an admin.
func AdminRequired() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "forbidden",
			})
		}
		return c.Next()
	}
}
