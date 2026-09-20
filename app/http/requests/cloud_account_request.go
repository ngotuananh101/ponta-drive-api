package requests

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/validation"
)

// TestCloudAccountRequest validates the payload for testing S3 connection credentials.
type TestCloudAccountRequest struct {
	Provider        string `form:"provider" json:"provider"`
	Endpoint        string `form:"endpoint" json:"endpoint"`
	Bucket          string `form:"bucket" json:"bucket"`
	Region          string `form:"region" json:"region"`
	AccessKeyID     string `form:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `form:"secret_access_key" json:"secret_access_key"`
	UsePathStyle    bool   `form:"use_path_style" json:"use_path_style"`
	PublicURL       string `form:"public_url" json:"public_url"`
}

func (r *TestCloudAccountRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *TestCloudAccountRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *TestCloudAccountRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"provider":          "required|in:s3,cloudflare_r2,minio,google_drive,onedrive",
		"bucket":            "required",
		"access_key_id":     "required",
		"secret_access_key": "required",
	}
}

func (r *TestCloudAccountRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *TestCloudAccountRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "provider", "bucket", "access_key_id", "secret_access_key")
}

func (r *TestCloudAccountRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// StoreCloudAccountRequest validates the payload for creating a new cloud account.
type StoreCloudAccountRequest struct {
	Name            string `form:"name" json:"name"`
	Provider        string `form:"provider" json:"provider"`
	Endpoint        string `form:"endpoint" json:"endpoint"`
	Bucket          string `form:"bucket" json:"bucket"`
	Region          string `form:"region" json:"region"`
	AccessKeyID     string `form:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `form:"secret_access_key" json:"secret_access_key"`
	UsePathStyle    bool   `form:"use_path_style" json:"use_path_style"`
	PublicURL       string `form:"public_url" json:"public_url"`
	IsDefault       bool   `form:"is_default" json:"is_default"`
	TotalStorage    int64  `form:"total_storage" json:"total_storage"`
}

func (r *StoreCloudAccountRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *StoreCloudAccountRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *StoreCloudAccountRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"name":              "required|max_len:100",
		"provider":          "required|in:s3,cloudflare_r2,minio,google_drive,onedrive",
		"bucket":            "required",
		"access_key_id":     "required",
		"secret_access_key": "required",
	}
}

func (r *StoreCloudAccountRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *StoreCloudAccountRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "name", "provider", "bucket", "access_key_id", "secret_access_key")
}

func (r *StoreCloudAccountRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// UpdateCloudAccountRequest validates the payload for updating an existing cloud account.
type UpdateCloudAccountRequest struct {
	Name            string `form:"name" json:"name"`
	Endpoint        string `form:"endpoint" json:"endpoint"`
	Bucket          string `form:"bucket" json:"bucket"`
	Region          string `form:"region" json:"region"`
	AccessKeyID     string `form:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `form:"secret_access_key" json:"secret_access_key"`
	UsePathStyle    *bool  `form:"use_path_style" json:"use_path_style"`
	PublicURL       string `form:"public_url" json:"public_url"`
	IsDefault       *bool  `form:"is_default" json:"is_default"`
	IsActive        *bool  `form:"is_active" json:"is_active"`
	TotalStorage    *int64 `form:"total_storage" json:"total_storage"`
}

func (r *UpdateCloudAccountRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *UpdateCloudAccountRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *UpdateCloudAccountRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"name": "max_len:100",
	}
}

func (r *UpdateCloudAccountRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *UpdateCloudAccountRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "name")
}

func (r *UpdateCloudAccountRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}
