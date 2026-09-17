package controllers

import (
	"strings"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
)

type AuthController struct{}

func NewAuthController() *AuthController {
	return &AuthController{}
}

func (c *AuthController) Login(ctx http.Context) http.Response {
	login := strings.TrimSpace(ctx.Request().Input("login"))
	if login == "" {
		login = strings.TrimSpace(ctx.Request().Input("username"))
	}
	password := ctx.Request().Input("password")

	if login == "" || password == "" {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": "Username/email and password are required",
		})
	}

	var user models.User
	err := facades.Orm().Query().
		Where("username = ? OR email = ?", login, login).
		First(&user)
	if err != nil || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "Invalid username or password",
		})
	}

	if !facades.Hash().Check(password, user.Password) {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "Invalid username or password",
		})
	}

	token, err := facades.Auth(ctx).Login(&user)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": "Failed to generate token",
		})
	}

	ttl := facades.Config().GetInt("jwt.ttl", 60) * 60

	return ctx.Response().Success().Json(http.Json{
		"status": "success",
		"data": http.Json{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": ttl,
			"user":       user.ToResponse(),
		},
	})
}

func (c *AuthController) Me(ctx http.Context) http.Response {
	if val := ctx.Value("user"); val != nil {
		if user, ok := val.(models.User); ok {
			return ctx.Response().Success().Json(http.Json{
				"status": "success",
				"data":   user.ToResponse(),
			})
		}
	}

	var user models.User
	if err := facades.Auth(ctx).User(&user); err != nil || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "Unauthenticated",
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "success",
		"data":   user.ToResponse(),
	})
}

func (c *AuthController) Refresh(ctx http.Context) http.Response {
	token, err := facades.Auth(ctx).Refresh()
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": "Failed to refresh token",
		})
	}

	ttl := facades.Config().GetInt("jwt.ttl", 60) * 60

	return ctx.Response().Success().Json(http.Json{
		"status": "success",
		"data": http.Json{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": ttl,
		},
	})
}

func (c *AuthController) Logout(ctx http.Context) http.Response {
	if err := facades.Auth(ctx).Logout(); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": "Failed to logout",
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "success",
		"message": "Successfully logged out",
	})
}
