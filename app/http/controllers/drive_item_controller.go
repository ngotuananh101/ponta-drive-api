package controllers

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/helpers"
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
	parentIDResolved := false

	// First try parent_uuid (modern approach)
	if parentUUID := ctx.Request().Query("parent_uuid"); parentUUID != "" {
		var parentItem models.DriveItem
		if err := facades.Orm().Query().Where("uuid", parentUUID).Where("user_id", user.ID).Where("cloud_account_id", cloudAccountID).First(&parentItem); err == nil && parentItem.ID > 0 {
			parentID = &parentItem.ID
			parentIDResolved = true
		}
	}

	// Fall back to parent_id (numeric) if parent_uuid not provided or not found
	if !parentIDResolved {
		parentStr := ctx.Request().Query("parent_id")
		if parentStr != "" {
			if pid, err := strconv.ParseUint(parentStr, 10, 64); err == nil && pid > 0 {
				uPid := uint(pid)
				parentID = &uPid
			}
		}
	}

	search := ctx.Request().Query("search")
	itemType := ctx.Request().Query("type", "all")
	sortField := services.NormalizeSortField(ctx.Request().Query("sort", "name"))
	sortOrder := services.NormalizeSortOrder(ctx.Request().Query("order", "asc"))

	// Default 50, hard ceiling 100: an unbounded page would pull the whole
	// folder into memory on a single request.
	limit := ctx.Request().QueryInt("limit", 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	var cursor services.DriveCursor
	if raw := ctx.Request().Query("cursor"); raw != "" {
		parsed, err := services.DecodeDriveCursor(raw)
		if err != nil {
			return failResponse(ctx, http.StatusBadRequest, "drive.invalid_cursor", err)
		}
		cursor = parsed
	}

	// Ask for one row more than the page size: its presence is what proves a
	// next page exists, without a second COUNT query.
	items, err := c.service.ListItems(context.Background(), user.ID, cloudAccountID, parentID, search, itemType, sortField, sortOrder, cursor, limit+1)
	if err != nil {
		return failResponse(ctx, http.StatusInternalServerError, "drive.list_failed", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		// The cursor records the ordering it was built under, so a later
		// request with a different `sort` can tell the position is stale.
		encoded, err := services.EncodeDriveCursor(items[len(items)-1], sortField, sortOrder)
		if err != nil {
			return failResponse(ctx, http.StatusInternalServerError, "drive.list_failed", err)
		}
		nextCursor = encoded
	}

	data := make([]map[string]any, 0, len(items))
	for i := range items {
		// Load cloud account to get its UUID for the response
		var account models.CloudAccount
		if err := facades.Orm().Query().Where("id", items[i].CloudAccountID).First(&account); err == nil && account.UUID != "" {
			items[i].CloudAccountUUID = account.UUID
		}
		data = append(data, items[i].ToResponse())
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   data,
		"meta": http.Json{
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		},
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
		return failResponse(ctx, http.StatusBadRequest, "drive.invalid_request", err)
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	// Resolve parent: prefer parent_uuid if provided, fall back to parent_id
	var parentID *uint
	if req.ParentUUID != "" {
		var parentItem models.DriveItem
		if err := facades.Orm().Query().Where("uuid", req.ParentUUID).Where("user_id", user.ID).Where("cloud_account_id", req.CloudAccountID).First(&parentItem); err != nil || parentItem.ID == 0 {
			return failResponse(ctx, http.StatusBadRequest, "drive.parent_not_found", err)
		}
		parentID = &parentItem.ID
	} else {
		parentID = req.ParentID
	}

	folder, err := c.service.CreateFolder(context.Background(), user.ID, req.CloudAccountID, parentID, req.Name)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "drive.create_failed", err)
	}

	// Load cloud account to get its UUID for the response
	var account models.CloudAccount
	if err := facades.Orm().Query().Where("id", req.CloudAccountID).First(&account); err == nil && account.UUID != "" {
		folder.CloudAccountUUID = account.UUID
	}

	activityService := services.NewActivityService()
	activityService.LogSafe(user.ID, folder.CloudAccountID, "created_folder", folder.Name, &folder.UUID, helpers.GetClientIP(ctx), helpers.GetUserAgent(ctx), nil)

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

	// Load cloud account to get its UUID for the response
	var account models.CloudAccount
	if err := facades.Orm().Query().Where("id", item.CloudAccountID).First(&account); err == nil && account.UUID != "" {
		item.CloudAccountUUID = account.UUID
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   item.ToResponse(),
	})
}

// Breadcrumb returns the ancestor chain for a folder (root first, folder last).
//
// The drive URL carries the current folder's uuid so a reload stays put; on
// load the client resolves that uuid back into the full path with this call.
func (c *DriveItemController) Breadcrumb(ctx http.Context) http.Response {
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

	chain, err := c.service.GetBreadcrumb(context.Background(), user.ID, itemUUID)
	if err != nil {
		return failResponse(ctx, http.StatusNotFound, "drive.item_not_found", err)
	}

	data := make([]map[string]any, 0, len(chain))
	for i := range chain {
		// Load cloud account to get its UUID for the response
		var account models.CloudAccount
		if err := facades.Orm().Query().Where("id", chain[i].CloudAccountID).First(&account); err == nil && account.UUID != "" {
			chain[i].CloudAccountUUID = account.UUID
		}
		data = append(data, chain[i].ToResponse())
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   data,
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
		return failResponse(ctx, http.StatusBadRequest, "drive.invalid_request", err)
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

	// Resolve parent: prefer parent_uuid if provided, fall back to parent_id
	var parentID *uint
	if req.ParentUUID != "" {
		var parentItem models.DriveItem
		if errRes := facades.Orm().Query().Where("uuid", req.ParentUUID).Where("user_id", user.ID).First(&parentItem); errRes != nil || parentItem.ID == 0 {
			return failResponse(ctx, http.StatusBadRequest, "drive.parent_not_found", errRes)
		}
		parentID = &parentItem.ID
	} else {
		parentID = req.ParentID
	}

	// Capture the current name so a rename can be detected after the update:
	// UpdateItem mutates and returns the same pointer, so item.Name afterwards
	// already holds the new name.
	previousName := ""
	hasPrevious := false
	if existing, fetchErr := c.service.GetItemByUUID(context.Background(), user.ID, itemUUID); fetchErr == nil {
		previousName = existing.Name
		hasPrevious = true
	}

	item, err := c.service.UpdateItem(context.Background(), user.ID, itemUUID, name, parentID)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "drive.update_failed", err)
	}

	if shouldLogRename(hasPrevious, name, previousName) {
		activityService := services.NewActivityService()
		activityService.LogSafe(user.ID, item.CloudAccountID, "renamed", fmt.Sprintf("%s -> %s", previousName, name), &item.UUID, helpers.GetClientIP(ctx), helpers.GetUserAgent(ctx), nil)
	}

	// Load cloud account to get its UUID for the response
	var account models.CloudAccount
	if err := facades.Orm().Query().Where("id", item.CloudAccountID).First(&account); err == nil && account.UUID != "" {
		item.CloudAccountUUID = account.UUID
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   item.ToResponse(),
	})
}

// shouldLogRename reports whether a rename activity should be recorded.
//
// hasPrevious is false when the pre-update lookup failed; in that case the old
// name is unknown and no rename can be asserted. This guards against logging a
// "renamed" activity (with an empty previous name) when nothing was renamed.
func shouldLogRename(hasPrevious bool, newName, previousName string) bool {
	if !hasPrevious || newName == "" {
		return false
	}

	return newName != previousName
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
		return failResponse(ctx, http.StatusBadRequest, "drive.star_failed", err)
	}

	// Load cloud account to get its UUID for the response
	var account models.CloudAccount
	if err := facades.Orm().Query().Where("id", item.CloudAccountID).First(&account); err == nil && account.UUID != "" {
		item.CloudAccountUUID = account.UUID
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

	// Look the item up first so the audit entry can name what was deleted. A
	// lookup failure is not fatal here: DeleteItem below is the operation that
	// decides success, and it re-validates ownership.
	item, lookupErr := c.service.GetItemByUUID(context.Background(), user.ID, itemUUID)
	if lookupErr != nil {
		facades.Log().Warningf("[Drive] Could not load item %s before delete for user %d: %v", itemUUID, user.ID, lookupErr)
	}

	err := c.service.DeleteItem(context.Background(), user.ID, itemUUID, permanent)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "drive.delete_failed", err)
	}

	if item != nil {
		activityService := services.NewActivityService()
		action := "deleted"
		if permanent {
			action = "permanently_deleted"
		}
		activityService.LogSafe(user.ID, item.CloudAccountID, action, item.Name, &item.UUID, helpers.GetClientIP(ctx), helpers.GetUserAgent(ctx), map[string]any{"permanent": permanent})
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
			return failResponse(ctx, http.StatusBadRequest, "drive.download_failed", err)
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
		return failResponse(ctx, http.StatusBadRequest, "drive.download_failed", err)
	}

	// Load cloud account to get its UUID for the response
	var account models.CloudAccount
	if err := facades.Orm().Query().Where("id", item.CloudAccountID).First(&account); err == nil && account.UUID != "" {
		item.CloudAccountUUID = account.UUID
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