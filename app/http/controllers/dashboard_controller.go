package controllers

import (
	"context"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
)

type DashboardController struct {
	service *services.DashboardService
}

func NewDashboardController() *DashboardController {
	return &DashboardController{
		service: services.NewDashboardService(),
	}
}

func (c *DashboardController) Summary(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	summary, err := c.service.GetSummary(context.Background(), user.ID)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   summary,
	})
}
