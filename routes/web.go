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
	cloudAccountController := controllers.NewCloudAccountController()
	driveItemController := controllers.NewDriveItemController()
	uploadController := controllers.NewUploadController()
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

		router.Prefix("v1").Middleware(jwtAuth).Group(func(v1 route.Router) {
			v1.Prefix("cloud-accounts").Group(func(ca route.Router) {
				ca.Get("", cloudAccountController.Index)
				ca.Post("", cloudAccountController.Store)
				ca.Post("/test", cloudAccountController.Test)
				ca.Get("/{id}", cloudAccountController.Show)
				ca.Put("/{id}", cloudAccountController.Update)
				ca.Delete("/{id}", cloudAccountController.Destroy)
				ca.Post("/{id}/sync", cloudAccountController.Sync)
			})

			v1.Prefix("drive").Group(func(drive route.Router) {
				drive.Prefix("items").Group(func(items route.Router) {
					items.Get("", driveItemController.Index)
					items.Post("/folders", driveItemController.StoreFolder)
					items.Get("/{uuid}", driveItemController.Show)
					items.Get("/{uuid}/download", driveItemController.Download)
					items.Patch("/{uuid}", driveItemController.Update)
					items.Post("/{uuid}/star", driveItemController.Star)
					items.Delete("/{uuid}", driveItemController.Destroy)
				})

				drive.Prefix("upload").Group(func(upload route.Router) {
					upload.Post("/presigned", uploadController.InitiatePresigned)
					upload.Post("/presigned/complete", uploadController.CompletePresigned)

					upload.Prefix("multipart").Group(func(mp route.Router) {
						mp.Post("/init", uploadController.InitMultipart)
						mp.Post("/part", uploadController.UploadPart)
						mp.Post("/complete", uploadController.CompleteMultipart)
						mp.Post("/abort", uploadController.AbortMultipart)
					})
				})
			})
		})

		router.Get("/users", userController.Index)
	})
}
