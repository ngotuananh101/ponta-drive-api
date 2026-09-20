package controllers

import (
	"context"
	"io"
	"strconv"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/requests"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
)

type UploadController struct {
	driveService *services.CloudDriveService
}

func NewUploadController() *UploadController {
	return &UploadController{
		driveService: services.NewCloudDriveService(),
	}
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

	item, presignedResp, err := c.driveService.InitiatePresignedUpload(
		context.Background(),
		user.ID,
		req.CloudAccountID,
		req.ParentID,
		req.FileName,
		req.Size,
		req.MimeType,
	)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

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

	item, err := c.driveService.CompletePresignedUpload(context.Background(), user.ID, req.ItemUUID)
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

	session, err := c.driveService.InitiateMultipartUpload(
		context.Background(),
		user.ID,
		req.CloudAccountID,
		req.ParentID,
		req.FileName,
		req.Size,
		req.MimeType,
		req.ChunkSize,
	)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
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
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
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

	item, err := c.driveService.CompleteMultipartUpload(context.Background(), user.ID, req.SessionID)
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
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "ok",
		"message": facades.Lang(ctx).Get("upload.aborted"),
	})
}

