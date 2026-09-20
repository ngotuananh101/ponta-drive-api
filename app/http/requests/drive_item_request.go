package requests

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/validation"
)

// CreateFolderRequest validates creation of a new virtual directory.
type CreateFolderRequest struct {
	CloudAccountID uint   `form:"cloud_account_id" json:"cloud_account_id"`
	ParentID       *uint  `form:"parent_id" json:"parent_id"`
	Name           string `form:"name" json:"name"`
}

func (r *CreateFolderRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *CreateFolderRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *CreateFolderRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"cloud_account_id": "required",
		"name":             "required|max_len:255",
	}
}

func (r *CreateFolderRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *CreateFolderRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "cloud_account_id", "name")
}

func (r *CreateFolderRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}

// UpdateDriveItemRequest validates renaming or moving an item.
type UpdateDriveItemRequest struct {
	Name     string `form:"name" json:"name"`
	ParentID *uint  `form:"parent_id" json:"parent_id"`
}

func (r *UpdateDriveItemRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *UpdateDriveItemRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *UpdateDriveItemRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"name": "max_len:255",
	}
}

func (r *UpdateDriveItemRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *UpdateDriveItemRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "name")
}

func (r *UpdateDriveItemRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	return nil
}
