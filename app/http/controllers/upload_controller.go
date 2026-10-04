package controllers

import (
	"context"
	"io"
	"strconv"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/helpers"
	"ponta_drive/app/http/requests"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
)

const (
	errKeyUploadInitiateFailed = "upload.initiate_failed"
	errKeyUploadInvalidRequest = "upload.invalid_request"
	errKeyUploadCompleteFailed = "upload.complete_failed"
)

type UploadController struct {
	driveService *services.CloudDriveService
}

func NewUploadController() *UploadController {
	return &UploadController{
		driveService: services.NewCloudDriveService(),
	}
}

func (c *UploadController) resolveUploadTargets(ctx context.Context, userID uint, cloudAccountUUID, parentUUID string) (uint, *uint, error) {
	cloudAccountID, err := c.driveService.ResolveCloudAccountID(ctx, userID, cloudAccountUUID)
	if err != nil {
		return 0, nil, err
	}

	var parentID *uint
	if parentUUID != "" {
		pid, err := c.driveService.ResolveDriveItemID(ctx, userID, parentUUID)
		if err != nil {
			return 0, nil, err
		}
		parentID = &pid
	}

	return cloudAccountID, parentID, nil
}

func (c *UploadController) finalizeUpload(ctx http.Context, userID uint, item *models.DriveItem) http.Response {
	item.CloudAccountUUID = c.accountUUIDFor(item)

	activityService := services.NewActivityService()
	activityService.LogSafe(userID, item.CloudAccountID, "uploaded", item.Name, &item.UUID, helpers.GetClientIP(ctx), helpers.GetUserAgent(ctx), map[string]any{"size": item.Size, "mime": item.MimeType})

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data":   item.ToResponse(),
	})
}

// InitiatePresigned generates a presigned upload URL and registers a pending DriveItem.
func (c *UploadController) InitiatePresigned(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var req requests.PresignedUploadRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInvalidRequest, err)
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	cloudAccountID, parentID, err := c.resolveUploadTargets(context.Background(), user.ID, req.CloudAccountUUID, req.ParentUUID)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInitiateFailed, err)
	}

	item, presignedResp, err := c.driveService.InitiatePresignedUpload(
		context.Background(),
		user.ID,
		cloudAccountID,
		parentID,
		req.FileName,
		req.Size,
		req.MimeType,
	)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInitiateFailed, err)
	}

	item.CloudAccountUUID = req.CloudAccountUUID

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data": http.Json{
			"item_uuid":  item.UUID,
			"upload_url": presignedResp.URL,
			"method":     presignedResp.Method,
			"headers":    presignedResp.Headers,
			"expires_at": presignedResp.Expires,
			"item":       item.ToResponse(),
		},
	})
}

// CompletePresigned verifies the uploaded file in S3 and finalizes the DriveItem status to ready.
func (c *UploadController) CompletePresigned(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var req requests.CompletePresignedUploadRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInvalidRequest, err)
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	item, err := c.driveService.CompletePresignedUpload(context.Background(), user.ID, req.ItemUUID)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadCompleteFailed, err)
	}

	return c.finalizeUpload(ctx, user.ID, item)
}

// InitMultipart initiates a server-side multipart/chunked upload session.
func (c *UploadController) InitMultipart(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var req requests.InitMultipartUploadRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInvalidRequest, err)
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	cloudAccountID, parentID, err := c.resolveUploadTargets(context.Background(), user.ID, req.CloudAccountUUID, req.ParentUUID)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInitiateFailed, err)
	}

	session, err := c.driveService.InitiateMultipartUpload(
		context.Background(),
		user.ID,
		cloudAccountID,
		parentID,
		req.FileName,
		req.Size,
		req.MimeType,
		req.ChunkSize,
	)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInitiateFailed, err)
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data": http.Json{
			"session_id":   session.SessionID,
			"upload_id":    session.UploadID,
			"total_parts":  session.TotalParts,
			"chunk_size":   session.ChunkSize,
			"storage_path": session.StoragePath,
		},
	})
}

// UploadPart streams an uploaded chunk directly to S3 without buffering the whole file to disk.
func (c *UploadController) UploadPart(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	sessionID := ctx.Request().Input("session_id")
	if sessionID == "" {
		sessionID = ctx.Request().Query("session_id")
	}
	if sessionID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("upload.session_id_required"),
		})
	}

	partNumStr := ctx.Request().Input("part_number")
	if partNumStr == "" {
		partNumStr = ctx.Request().Query("part_number")
	}
	partNum, err := strconv.ParseInt(partNumStr, 10, 32)
	if err != nil || partNum <= 0 {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("upload.part_number_required"),
		})
	}

	originReq := ctx.Request().Origin()
	var reader io.Reader
	var partSize int64

	file, header, err := originReq.FormFile("file")
	if err == nil {
		defer file.Close()
		reader = file
		partSize = header.Size
	} else if originReq.Body != nil {
		reader = originReq.Body
		partSize = originReq.ContentLength
	}

	if reader == nil || partSize <= 0 {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("upload.payload_missing"),
		})
	}

	etag, _, err := c.driveService.UploadPart(context.Background(), user.ID, sessionID, int32(partNum), reader, partSize)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "upload.part_failed", err)
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "ok",
		"data": http.Json{
			"session_id":  sessionID,
			"part_number": partNum,
			"etag":        etag,
		},
	})
}

// CompleteMultipart completes the multipart upload on S3 and records the DriveItem in the database.
func (c *UploadController) CompleteMultipart(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	var req requests.CompleteMultipartUploadRequest
	errors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadInvalidRequest, err)
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	item, err := c.driveService.CompleteMultipartUpload(context.Background(), user.ID, req.SessionID)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, errKeyUploadCompleteFailed, err)
	}

	return c.finalizeUpload(ctx, user.ID, item)
}

// AbortMultipart aborts an in-progress multipart upload on S3 and deletes the session.
func (c *UploadController) AbortMultipart(ctx http.Context) http.Response {
	user, ok := ctx.Value("user").(models.User)
	if !ok || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("common.unauthorized"),
		})
	}

	sessionID := ctx.Request().Input("session_id")
	if sessionID == "" {
		sessionID = ctx.Request().Query("session_id")
	}
	if sessionID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("upload.session_id_required"),
		})
	}

	err := c.driveService.AbortMultipartUpload(context.Background(), user.ID, sessionID)
	if err != nil {
		return failResponse(ctx, http.StatusBadRequest, "upload.abort_failed", err)
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("upload.session_aborted"),
	})
}

// accountUUIDFor resolves the public account uuid for an item, best-effort.
//
// The session row holds only the internal numeric account id; we read the
// account's uuid in a single cheap query so the response can carry the public
// identifier without exposing the database pk.
func (c *UploadController) accountUUIDFor(item *models.DriveItem) string {
	if item == nil || item.CloudAccountID == 0 {
		return ""
	}

	var account models.CloudAccount
	if err := facades.Orm().Query().Where("id", item.CloudAccountID).Select("uuid").First(&account); err == nil {
		return account.UUID
	}

	return ""
}
