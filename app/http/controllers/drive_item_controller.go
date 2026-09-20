package controllers

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/requests"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
)

type DriveItemController struct {
	service *services.CloudDriveService
}

func NewDriveItemController() *DriveItemController {
	return &DriveItemController{
		service: services.NewCloudDriveService(),
	}
}

// Index lists drive items (files & folders) in a directory.
func (c *DriveItemController) Index(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	cloudAccountID := uint(ctx.Request().QueryInt("cloud_account_id", 0))
	if cloudAccountID == 0 {
		// Try default cloud account
		var defaultAcc models.CloudAccount
		_ = facades.Orm().Query().
			Where("user_id", user.ID).
			Where("is_default", true).
			First(&defaultAcc)
		if defaultAcc.ID > 0 {
			cloudAccountID = defaultAcc.ID
		} else {
			return ctx.Response().Json(http.StatusBadRequest, http.Json{
				"status":  "error",
				"message": facades.Lang(ctx).Get("drive.cloud_account_required"),
			})
		}
	}

	var parentID *uint
	parentStr := ctx.Request().Query("parent_id")
	if parentStr != "" {
		if pid, err := strconv.ParseUint(parentStr, 10, 64); err == nil && pid > 0 {
			uPid := uint(pid)
			parentID = &uPid
		}
	}

	search := ctx.Request().Query("search")
	itemType := ctx.Request().Query("type", "all")
	sortField := ctx.Request().Query("sort", "name")
	sortOrder := ctx.Request().Query("order", "asc")

	items, err := c.service.ListItems(context.Background(), user.ID, cloudAccountID, parentID, search, itemType, sortField, sortOrder)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("drive.list_failed") + ": " + err.Error(),
		})
	}

	data := make([]map[string]any, 0, len(items))
	for _, item := range items {
		data = append(data, item.ToResponse())
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   data,
	})
}

// StoreFolder creates a new virtual folder.
func (c *DriveItemController) StoreFolder(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var req requests.CreateFolderRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	folder, err := c.service.CreateFolder(context.Background(), user.ID, req.CloudAccountID, req.ParentID, req.Name)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return ctx.Response().Json(http.StatusCreated, http.Json{
		"status": "ok",
		"data":   folder.ToResponse(),
	})
}

// Show returns metadata of a single item by UUID.
func (c *DriveItemController) Show(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	itemUUID := ctx.Request().Route("uuid")
	if itemUUID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.uuid_required"),
		})
	}

	item, err := c.service.GetItemByUUID(context.Background(), user.ID, itemUUID)
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("drive.item_not_found"),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   item.ToResponse(),
	})
}

// Update updates an item's name or parent directory.
func (c *DriveItemController) Update(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	itemUUID := ctx.Request().Route("uuid")
	if itemUUID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.uuid_required"),
		})
	}

	var req requests.UpdateDriveItemRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	name := req.Name
	if name == "" {
		name = ctx.Request().Input("name")
	}

	item, err := c.service.UpdateItem(context.Background(), user.ID, itemUUID, name, req.ParentID)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   item.ToResponse(),
	})
}

// Star toggles the star flag on an item.
func (c *DriveItemController) Star(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	itemUUID := ctx.Request().Route("uuid")
	if itemUUID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.uuid_required"),
		})
	}

	item, err := c.service.ToggleStar(context.Background(), user.ID, itemUUID)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   item.ToResponse(),
	})
}

// Destroy deletes an item.
func (c *DriveItemController) Destroy(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	itemUUID := ctx.Request().Route("uuid")
	if itemUUID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.uuid_required"),
		})
	}

	permanent := ctx.Request().QueryBool("permanent", false)

	err := c.service.DeleteItem(context.Background(), user.ID, itemUUID, permanent)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("drive.item_deleted"),
	})
}

// Download handles item download either by redirecting to presigned URL, returning JSON, or proxying the stream.
func (c *DriveItemController) Download(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	itemUUID := ctx.Request().Route("uuid")
	if itemUUID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.uuid_required"),
		})
	}

	mode := ctx.Request().Query("mode", "redirect")

	if mode == "stream" {
		stream, item, err := c.service.GetItemStream(context.Background(), user.ID, itemUUID)
		if err != nil {
			return ctx.Response().Json(http.StatusBadRequest, http.Json{
				"status":  "error",
				"message": err.Error(),
			})
		}

		mimeType := item.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		ctx.Response().Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", item.Name))
		ctx.Response().Header("Content-Type", mimeType)
		if item.Size > 0 {
			ctx.Response().Header("Content-Length", strconv.FormatInt(item.Size, 10))
		}

		return ctx.Response().Stream(http.StatusOK, func(w http.StreamWriter) error {
			defer stream.Close()
			_, err := io.Copy(w, stream)
			return err
		})
	}

	downloadURL, item, err := c.service.GetPresignedDownloadURL(context.Background(), user.ID, itemUUID, 15*time.Minute)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	if mode == "json" {
		return ctx.Response().Success().Json(http.Json{
			"status": "ok",
			"data": http.Json{
				"download_url": downloadURL,
				"item":         item.ToResponse(),
			},
		})
	}

	return ctx.Response().Redirect(http.StatusFound, downloadURL)
}

