package controllers

import (
	"context"
	"fmt"
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

// dispatchSync marks the account as syncing, records the activity, and hands the
// bucket scan to the queue. It is shared by Sync (the explicit endpoint) and
// Store (the automatic first sync) so both behave identically.
//
// With the "sync" queue driver the job runs inline and returns only when the
// scan finishes, so the account is already back to idle/error by the time this
// returns. With "database" the job is stored and a worker picks it up, leaving
// the account in "syncing" until the worker updates it.
func (c *CloudAccountController) dispatchSync(ctx http.Context, userID uint, account *models.CloudAccount, parentID *uint) {
	_, _ = facades.Orm().Query().Model(&models.CloudAccount{}).Where("id", account.ID).Update(map[string]any{"sync_status": "syncing"})

	activityService := services.NewActivityService()
	activityService.LogSafe(userID, account.ID, "sync_started", account.Name, nil, helpers.GetClientIP(ctx), helpers.GetUserAgent(ctx), nil)

	job := &jobs.SyncS3BucketJob{}
	err := facades.Queue().Job(job, []queue.Arg{
		{Value: userID, Type: "uint"},
		{Value: account.ID, Type: "uint"},
		{Value: parentID, Type: "*uint"},
	}).Dispatch()
	if err != nil {
		// Dispatch only fails when the queue backend rejected the job, so it
		// never ran and the account would be stuck at "syncing". Release it to
		// "error" and log the cause (never surfaced to the client). When the job
		// did run and its scan failed, its own defer already set "error", so
		// this is a harmless re-set.
		facades.Log().Errorf("[cloud] sync dispatch failed account=%d: %v", account.ID, err)
		_, _ = facades.Orm().Query().Model(&models.CloudAccount{}).Where("id", account.ID).Update(map[string]any{"sync_status": "error"})
	}
}

// Index lists all cloud accounts belonging to the authenticated user.
func (c *CloudAccountController) Index(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return failResponse(ctx, http.StatusUnauthorized, "common.unauthorized", nil)
	}

	var accounts []models.CloudAccount
	err := facades.Orm().Query().
		Where("user_id", user.ID).
		Order("is_default desc, id desc").
		Get(&accounts)
	if err != nil {
		return failResponse(ctx, http.StatusInternalServerError, "cloud.fetch_failed", err)
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
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_request", err)
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

	// Bound the outbound calls so a slow or unreachable endpoint fails with a
	// logged, localized error instead of running past the global HTTP timeout,
	// which would abort the request with a bare 408 and write nothing to the log.
	testCtx, cancel := context.WithTimeout(ctx.Context(), 15*time.Second)
	defer cancel()

	driver, err := c.factory.BuildDriver(testCtx, req.Provider, creds)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "cloud.driver_init_failed", fmt.Errorf("provider=%s: %w", req.Provider, err))
	}

	// Validate bucket connection by listing up to 1 item
	_, err = driver.ListObjects(testCtx, "", "", 1)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "cloud.connection_test_failed", fmt.Errorf("provider=%s endpoint=%s bucket=%s: %w", req.Provider, req.Endpoint, req.Bucket, err))
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
		return failResponse(ctx, http.StatusUnauthorized, "common.unauthorized", nil)
	}

	var req requests.StoreCloudAccountRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_request", err)
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
		return failResponse(ctx, http.StatusInternalServerError, "cloud.encrypt_failed", err)
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
		return failResponse(ctx, http.StatusInternalServerError, "cloud.save_failed", err)
	}

	// Scan the bucket right away so a freshly connected account is not empty
	// when the user opens it. With the sync driver this runs inline and the
	// reload below reflects the finished state; with database it stays
	// "syncing" until the worker completes.
	c.dispatchSync(ctx, user.ID, &account, nil)

	// Reload into a fresh value so the response carries the sync_status the
	// dispatch produced rather than the zero value from before it ran.
	var fresh models.CloudAccount
	if err := facades.Orm().Query().Where("id", account.ID).First(&fresh); err == nil {
		account = fresh
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
		return failResponse(ctx, http.StatusUnauthorized, "common.unauthorized", nil)
	}

	uuid := ctx.Request().Route("uuid")
	if uuid == "" {
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_account_id", nil)
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("uuid", uuid).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.UUID == "" {
		return failResponse(ctx, http.StatusNotFound, "cloud.not_found", err)
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
		return failResponse(ctx, http.StatusUnauthorized, "common.unauthorized", nil)
	}

	uuid := ctx.Request().Route("uuid")
	if uuid == "" {
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_account_id", nil)
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("uuid", uuid).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.UUID == "" {
		return failResponse(ctx, http.StatusNotFound, "cloud.not_found", err)
	}

	var req requests.UpdateCloudAccountRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_request", err)
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
		if err := account.SetCredentials(existingCreds); err != nil {
			return failResponse(ctx, http.StatusInternalServerError, "cloud.encrypt_failed", err)
		}
	}

	if err := facades.Orm().Query().Save(&account); err != nil {
		return failResponse(ctx, http.StatusInternalServerError, "cloud.update_failed", err)
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
		return failResponse(ctx, http.StatusUnauthorized, "common.unauthorized", nil)
	}

	uuid := ctx.Request().Route("uuid")
	if uuid == "" {
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_account_id", nil)
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("uuid", uuid).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.UUID == "" {
		return failResponse(ctx, http.StatusNotFound, "cloud.not_found", err)
	}

	if _, err := facades.Orm().Query().Delete(&account); err != nil {
		return failResponse(ctx, http.StatusInternalServerError, "cloud.delete_failed", err)
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
		return failResponse(ctx, http.StatusUnauthorized, "common.unauthorized", nil)
	}

	uuid := ctx.Request().Route("uuid")
	if uuid == "" {
		return failResponse(ctx, http.StatusBadRequest, "common.invalid_account_id", nil)
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("uuid", uuid).
		Where("user_id", user.ID).
		First(&account)
	if err != nil || account.UUID == "" {
		return failResponse(ctx, http.StatusNotFound, "cloud.not_found", err)
	}

	var parentID *uint
	if parentStr := ctx.Request().Query("parent_id"); parentStr != "" {
		if pid, err := strconv.ParseUint(parentStr, 10, 64); err == nil && pid > 0 {
			uPid := uint(pid)
			parentID = &uPid
		}
	}

	c.dispatchSync(ctx, user.ID, &account, parentID)

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("cloud.sync_started"),
	})
}
