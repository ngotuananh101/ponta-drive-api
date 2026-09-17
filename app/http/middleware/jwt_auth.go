package middleware

import (
	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
)

type JwtAuth struct{}

func (m *JwtAuth) Signature() string {
	return "jwt_auth"
}

func (m *JwtAuth) Handle(ctx http.Context) {
	token := ctx.Request().Header("Authorization")
	if token == "" {
		_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "Authorization header is required",
		}).Abort()
		return
	}

	if _, err := facades.Auth(ctx).Parse(token); err != nil {
		_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "Invalid or expired token",
		}).Abort()
		return
	}

	var user models.User
	if err := facades.Auth(ctx).User(&user); err != nil || user.ID == 0 {
		_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "User not found",
		}).Abort()
		return
	}

	ctx.WithValue("user", user)
	ctx.Request().Next()
}
