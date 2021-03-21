package app

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/womat/debug"
	"net/http"
	"relayhat/pkg/app/relay"
	"relayhat/pkg/app/validate"
)

// runWebServer starts the applications web server and listens for web requests.
//  It's designed to run in a separate go function to not block the main go function.
//  e.g.: go runWebServer()
//  See app.Run()
func (app *App) runWebServer() {
	err := app.web.Listen(app.urlParsed.Host)
	debug.FatalLog.Print(err)
}

// HandleRelayList is the list relay api endpoint.
func (app *App) HandleRelayList() fiber.Handler {
	type Response struct{ relay.AllHTTPBody }
	type Request struct{ relay.AllHTTPBody }

	return func(ctx *fiber.Ctx) error {
		u, err := app.relays.All()
		if err != nil {
			return fiber.ErrNotFound
		}

		return ctx.Status(http.StatusCreated).JSON(u)
	}
}

// HandleRelayGet is the get relay details api endpoint.
func (app *App) HandleRelayGet() fiber.Handler {
	type Response struct{ relay.HTTPBody }
	type Request struct{ relay.HTTPBody }

	return func(ctx *fiber.Ctx) error {
		id := ctx.Params("relay")
		u, err := app.relays.Get(id)
		if err != nil {
			return fiber.ErrNotFound
		}

		return ctx.Status(http.StatusCreated).JSON(Response{
			u,
		})
	}
}

// HandleRelayUpdate is the update relay api endpoint.
func (app *App) HandleRelayUpdate() fiber.Handler {
	type Response struct{ relay.HTTPBody }
	type Request struct{ relay.HTTPBody }

	return func(ctx *fiber.Ctx) error {
		id := ctx.Params("relay")

		u, err := app.relays.Get(id)
		if err != nil {
			return fiber.ErrNotFound
		}

		// parse request
		if err := ctx.BodyParser(&u); err != nil {
			ctx.Status(http.StatusPreconditionFailed)
			return err
		}

		// validate request
		if !validate.IsStructValid(u) {
			return ctx.
				Status(fiber.StatusUnprocessableEntity).
				JSON(validate.Struct(u))
		}

		u, err = app.relays.Update(id, u)
		if err != nil {
			if errors.Is(err, relay.ErrNotFound) {
				return fiber.ErrNotFound
			}
			ctx.Status(fiber.StatusBadRequest)
			return err
		}

		// return updated relay
		return ctx.Status(http.StatusCreated).JSON(Response{
			u,
		})
	}
}
