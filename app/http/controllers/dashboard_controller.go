package controllers

import (
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

	// Pass the request context so a client disconnect or timeout cancels the
	// underlying queries instead of leaving them running.
	summary, err := c.service.GetSummary(ctx.Context(), user.ID)
	if err != nil {
		// Log the underlying cause, but never surface it to the client: DB errors
		// can leak schema and query details.
		facades.Log().Errorf("[Dashboard] Failed to build summary for user %d: %v", user.ID, err)
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.fetch_failed"),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   summary,
	})
}
