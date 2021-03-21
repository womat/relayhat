package app

import (
	"github.com/gofiber/fiber/v2"
	"strings"
)

const VERSION = "0.0.1+20210320"
const MODULE = "relayhat"

// HandleVersion is the get application version web handler.
func (app *App) HandleVersion() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(fiber.Map{
			"version":     VERSION,
			"description": MODULE,
			"about":       Version(),
		})
	}
}

// Version is the get application version as string.
func Version() string {
	return strings.TrimSpace(MODULE + " V" + strings.Split(VERSION, "+")[0])
}
