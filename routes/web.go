package routes

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/route"
	"github.com/goravel/framework/support"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/controllers"
)

func Web() {
	facades.Route().Get("/", func(ctx http.Context) http.Response {
		return ctx.Response().View().Make("welcome.tmpl", map[string]any{
			"version": support.Version,
		})
	})

	facades.Route().Static("public", "./public")

	userController := controllers.NewUserController()
	facades.Route().Prefix("api").Group(func(router route.Router) {
		router.Get("/ping", func(ctx http.Context) http.Response {
			return ctx.Response().Success().Json(http.Json{
				"status":  "ok",
				"message": "pong from  Backend",
				"version": support.Version,
			})
		})
		router.Get("/users", userController.Index)
	})
}
