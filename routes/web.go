package routes

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/route"
	"github.com/goravel/framework/support"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/controllers"
	"ponta_drive/app/http/middleware"
)

func Web() {
	facades.Route().Get("/", func(ctx http.Context) http.Response {
		return ctx.Response().View().Make("welcome.tmpl", map[string]any{
			"version": support.Version,
		})
	})

	facades.Route().Static("public", "./public")
	facades.Route().StaticFile("favicon.ico", "./public/favicon.ico")

	userController := controllers.NewUserController()
	authController := controllers.NewAuthController()
	jwtAuth := &middleware.JwtAuth{}
	setLocale := &middleware.SetLocale{}

	facades.Route().Prefix("api").Middleware(setLocale).Group(func(router route.Router) {
		router.Get("/ping", func(ctx http.Context) http.Response {
			return ctx.Response().Success().Json(http.Json{
				"status":  "ok",
				"message": "pong from Goravel Backend",
				"version": support.Version,
			})
		})

		router.Prefix("auth").Group(func(authRouter route.Router) {
			authRouter.Post("/login", authController.Login)
			authRouter.Post("/forgot-password", authController.ForgotPassword)
			authRouter.Post("/reset-password", authController.ResetPassword)

			authRouter.Middleware(jwtAuth).Group(func(protected route.Router) {
				protected.Get("/me", authController.Me)
				protected.Post("/refresh", authController.Refresh)
				protected.Post("/logout", authController.Logout)
			})
		})

		router.Get("/users", userController.Index)
	})
}
