package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

// SanitizeLegacyServerErrors preserves the old management response shape while
// preventing database, filesystem, and provider details from crossing the API
// boundary. V2 handlers already return their typed, sanitized error envelope.
func SanitizeLegacyServerErrors() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		status := c.Response().StatusCode()
		if err != nil {
			var fiberError *fiber.Error
			if errors.As(err, &fiberError) {
				status = fiberError.Code
			} else {
				status = fiber.StatusInternalServerError
			}
		}
		if status < fiber.StatusInternalServerError {
			return err
		}
		return c.Status(status).JSON(fiber.Map{
			"error": true,
			"msg":   "The server could not complete that request.",
		})
	}
}
