package middlewares

import (
	"os"

	"github.com/gofiber/fiber/v2"
)

func Auth(c *fiber.Ctx) error {
	authKey := c.Query(`auth`)

	if authKey != os.Getenv("API_KEY") {
		return c.SendStatus(401)
	}

	return c.Next()
}
