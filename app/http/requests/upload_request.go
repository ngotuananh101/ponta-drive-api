package requests

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/validation"
)

// PresignedUploadRequest validates the payload for initiating direct-to-S3 presigned uploads.
type PresignedUploadRequest struct {
	CloudAccountID uint   `form:"cloud_account_id" json:"cloud_account_id"`
	ParentID       *uint  `form:"parent_id" json:"parent_id"`
	FileName       string `form:"file_name" json:"file_name"`
	Size           int64  `form:"size" json:"size"`
	MimeType       string `form:"mime_type" json:"mime_type"`
}

func (r *PresignedUploadRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *PresignedUploadRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *PresignedUploadRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"cloud_account_id": "required",
		"file_name":        "required|max_len:255",
		"size":             "required|gt:0",
	}
}

func (r *PresignedUploadRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *PresignedUploadRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "cloud_account_id", "file_name", "size")
}

func (r *PresignedUploadRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// CompletePresignedUploadRequest validates the confirmation of a completed presigned upload.
type CompletePresignedUploadRequest struct {
	ItemUUID string `form:"item_uuid" json:"item_uuid"`
}

func (r *CompletePresignedUploadRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *CompletePresignedUploadRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *CompletePresignedUploadRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"item_uuid": "required|max_len:36",
	}
}

func (r *CompletePresignedUploadRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *CompletePresignedUploadRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "item_uuid")
}

func (r *CompletePresignedUploadRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// InitMultipartUploadRequest validates payload for initializing a multipart/chunked upload session.
type InitMultipartUploadRequest struct {
	CloudAccountID uint   `form:"cloud_account_id" json:"cloud_account_id"`
	ParentID       *uint  `form:"parent_id" json:"parent_id"`
	FileName       string `form:"file_name" json:"file_name"`
	Size           int64  `form:"size" json:"size"`
	MimeType       string `form:"mime_type" json:"mime_type"`
	ChunkSize      int64  `form:"chunk_size" json:"chunk_size"`
}

func (r *InitMultipartUploadRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *InitMultipartUploadRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *InitMultipartUploadRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"cloud_account_id": "required",
		"file_name":        "required|max_len:255",
		"size":             "required|gt:0",
	}
}

func (r *InitMultipartUploadRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *InitMultipartUploadRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "cloud_account_id", "file_name", "size")
}

func (r *InitMultipartUploadRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// CompleteMultipartUploadRequest validates payload for completing a multipart upload.
type CompleteMultipartUploadRequest struct {
	SessionID string `form:"session_id" json:"session_id"`
}

func (r *CompleteMultipartUploadRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *CompleteMultipartUploadRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *CompleteMultipartUploadRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"session_id": "required|max_len:36",
	}
}

func (r *CompleteMultipartUploadRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *CompleteMultipartUploadRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "session_id")
}

func (r *CompleteMultipartUploadRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// AbortMultipartUploadRequest validates payload for aborting a multipart upload.
type AbortMultipartUploadRequest struct {
	SessionID string `form:"session_id" json:"session_id"`
}

func (r *AbortMultipartUploadRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *AbortMultipartUploadRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *AbortMultipartUploadRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"session_id": "required|max_len:36",
	}
}

func (r *AbortMultipartUploadRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *AbortMultipartUploadRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "session_id")
}

func (r *AbortMultipartUploadRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

