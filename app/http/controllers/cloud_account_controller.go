package controllers

import (
	"context"
	"strconv"
	"time"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/queue"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/helpers"
	"ponta_drive/app/http/requests"
	"ponta_drive/app/jobs"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
	"ponta_drive/app/services/storage"
)

type CloudAccountController struct {
	factory     *storage.DriverFactory
	credService *storage.CredentialService
}

func NewCloudAccountController() *CloudAccountController {
	return &CloudAccountController{
		factory:     storage.NewDriverFactory(),
		credService: storage.NewCredentialService(),
	}
}

// Index lists all cloud accounts belonging to the authenticated user.
func (c *CloudAccountController) Index(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var accounts []models.CloudAccount
	err := facades.Orm().Query().
		Where("user_id", user.ID).
		Order("is_default desc, id desc").
		Get(&accounts)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.fetch_failed"),
		})
	}

	data := make([]map[string]any, 0, len(accounts))
	for _, acc := range accounts {
		data = append(data, acc.ToResponse())
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   data,
	})
}

// Test tests connectivity using provided credentials without saving to the database.
func (c *CloudAccountController) Test(ctx http.Context) http.Response {
	var req requests.TestCloudAccountRequest
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

	creds := &models.S3Credentials{
		Endpoint:        req.Endpoint,
		Bucket:          req.Bucket,
		Region:          req.Region,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
		UsePathStyle:    req.UsePathStyle,
		PublicURL:       req.PublicURL,
	}

	driver, err := c.factory.BuildDriver(context.Background(), req.Provider, creds)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.driver_init_failed"),
		})
	}

	// Validate bucket connection by listing up to 1 item
	_, err = driver.ListObjects(context.Background(), "", "", 1)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.connection_test_failed"),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("cloud.connection_success"),
	})
}

// Store creates a new cloud account with encrypted credentials.
func (c *CloudAccountController) Store(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var req requests.StoreCloudAccountRequest
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

	creds := &models.S3Credentials{
		Endpoint:        req.Endpoint,
		Bucket:          req.Bucket,
		Region:          req.Region,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
		UsePathStyle:    req.UsePathStyle,
		PublicURL:       req.PublicURL,
	}

	encryptedCreds, err := c.credService.EncryptCredentials(creds)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.encrypt_failed"),
		})
	}

	// If marked as default, unset existing default accounts
	if req.IsDefault {
		_, _ = facades.Orm().Query().
			Where("user_id", user.ID).
			Update("is_default", false)
	}

	account := models.CloudAccount{
		UserID:       user.ID,
		Name:         req.Name,
		Provider:     req.Provider,
		Credentials:  encryptedCreds,
		IsDefault:    req.IsDefault,
		IsActive:     true,
		TotalStorage: req.TotalStorage,
	}

	if err := facades.Orm().Query().Create(&account); err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.save_failed"),
		})
	}

	return ctx.Response().Json(http.StatusCreated, http.Json{
		"status": "ok",
		"data":   account.ToResponse(),
	})
}

// Show returns details of a single cloud account.
func (c *CloudAccountController) Show(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	id := ctx.Request().RouteInt("id")
	if id <= 0 {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.invalid_account_id"),
		})
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("id", id).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.ID == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.not_found"),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   account.ToResponse(),
	})
}

// Update modifies an existing cloud account.
func (c *CloudAccountController) Update(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	id := ctx.Request().RouteInt("id")
	if id <= 0 {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.invalid_account_id"),
		})
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("id", id).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.ID == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.not_found"),
		})
	}

	var req requests.UpdateCloudAccountRequest
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

	if req.Name != "" {
		account.Name = req.Name
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			_, _ = facades.Orm().Query().
				Where("user_id", user.ID).
				Update("is_default", false)
		}
		account.IsDefault = *req.IsDefault
	}
	if req.IsActive != nil {
		account.IsActive = *req.IsActive
	}
	if req.TotalStorage != nil {
		account.TotalStorage = *req.TotalStorage
	}

	// If credential fields are partially or fully supplied, update credentials
	if req.AccessKeyID != "" || req.SecretAccessKey != "" || req.Bucket != "" || req.Endpoint != "" {
		existingCreds, _ := account.GetCredentials()
		if existingCreds == nil {
			existingCreds = &models.S3Credentials{}
		}
		if req.Bucket != "" {
			existingCreds.Bucket = req.Bucket
		}
		if req.Endpoint != "" {
			existingCreds.Endpoint = req.Endpoint
		}
		if req.Region != "" {
			existingCreds.Region = req.Region
		}
		if req.AccessKeyID != "" {
			existingCreds.AccessKeyID = req.AccessKeyID
		}
		if req.SecretAccessKey != "" {
			existingCreds.SecretAccessKey = req.SecretAccessKey
		}
		if req.UsePathStyle != nil {
			existingCreds.UsePathStyle = *req.UsePathStyle
		}
		if req.PublicURL != "" {
			existingCreds.PublicURL = req.PublicURL
		}
		_ = account.SetCredentials(existingCreds)
	}

	if err := facades.Orm().Query().Save(&account); err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.update_failed"),
		})
	}

	c.factory.InvalidateCache(account.ID)

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   account.ToResponse(),
	})
}

// Destroy soft-deletes a cloud account.
func (c *CloudAccountController) Destroy(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	id := ctx.Request().RouteInt("id")
	if id <= 0 {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.invalid_account_id"),
		})
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("id", id).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.ID == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.not_found"),
		})
	}

	if _, err := facades.Orm().Query().Delete(&account); err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.delete_failed"),
		})
	}

	c.factory.InvalidateCache(account.ID)

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("cloud.deleted"),
	})
}

// Sync triggers a background or synchronous scan and synchronization of the cloud storage bucket.
func (c *CloudAccountController) Sync(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	id := ctx.Request().RouteInt("id")
	if id <= 0 {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.invalid_account_id"),
		})
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("id", id).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.ID == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("cloud.not_found"),
		})
	}

	var parentID *uint
	if parentStr := ctx.Request().Query("parent_id"); parentStr != "" {
		if pid, err := strconv.ParseUint(parentStr, 10, 64); err == nil && pid > 0 {
			uPid := uint(pid)
			parentID = &uPid
		}
	}

	_, _ = facades.Orm().Query().Where("id", account.ID).Update(map[string]any{"sync_status": "syncing"})
	activityService := services.NewActivityService()
	_ = activityService.Log(user.ID, account.ID, "sync_started", account.Name, nil, helpers.GetClientIP(ctx), helpers.GetUserAgent(ctx), nil)

	// Dispatch sync job via Queue facade
	job := &jobs.SyncS3BucketJob{}
	err = facades.Queue().Job(job, []queue.Arg{
		{Value: user.ID, Type: "uint"},
		{Value: account.ID, Type: "uint"},
		{Value: parentID, Type: "*uint"},
	}).Dispatch()
	if err != nil {
		// Fallback: run scan directly
		driveService := services.NewCloudDriveService()
		_, _ = driveService.ScanBucket(context.Background(), user.ID, account.ID, parentID)
		// The job never ran, so release the account from the "syncing" state here.
		_, _ = facades.Orm().Query().Where("id", account.ID).Update(map[string]any{"sync_status": "idle", "last_synced_at": time.Now()})
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("cloud.sync_started"),
	})
}
