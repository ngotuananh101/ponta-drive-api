package models

import (
	"github.com/goravel/framework/database/orm"
)

// DriveItem type constants
const (
	ItemTypeFolder = "folder"
	ItemTypeFile   = "file"
)

// DriveItem status constants
const (
	ItemStatusReady     = "ready"
	ItemStatusUploading = "uploading"
	ItemStatusSyncing   = "syncing"
	ItemStatusFailed    = "failed"
)

// DriveItem represents a file or folder in the virtual hierarchy (spec 2.2).
type DriveItem struct {
	orm.Model
	UUID           string `gorm:"column:uuid;size:36;uniqueIndex;not null" json:"uuid"`
	UserID         uint   `gorm:"column:user_id;index;not null" json:"user_id"`
	CloudAccountID uint   `gorm:"column:cloud_account_id;index;not null" json:"cloud_account_id"`
	ParentID       *uint  `gorm:"column:parent_id;index" json:"parent_id"`
	Name           string `gorm:"column:name;size:255;not null" json:"name"`
	Type           string `gorm:"column:type;size:20;index;not null" json:"type"`
	MimeType       string `gorm:"column:mime_type;size:100" json:"mime_type"`
	Size           int64  `gorm:"column:size;default:0" json:"size"`
	Extension      string `gorm:"column:extension;size:50" json:"extension"`
	StoragePath    string `gorm:"column:storage_path;type:text" json:"storage_path"`
	ETag           string `gorm:"column:etag;size:100" json:"etag"`
	IsStarred      bool   `gorm:"column:is_starred;index;default:false" json:"is_starred"`
	Status         string `gorm:"column:status;size:20;index;default:ready" json:"status"`
	orm.SoftDeletes
}

// IsFolder returns true if the item is a folder.
func (d *DriveItem) IsFolder() bool {
	return d.Type == ItemTypeFolder
}

// IsFile returns true if the item is a file.
func (d *DriveItem) IsFile() bool {
	return d.Type == ItemTypeFile
}

// ToResponse returns a serialized map suitable for API responses.
func (d *DriveItem) ToResponse() map[string]any {
	return map[string]any{
		"id":               d.ID,
		"uuid":             d.UUID,
		"user_id":          d.UserID,
		"cloud_account_id": d.CloudAccountID,
		"parent_id":        d.ParentID,
		"name":             d.Name,
		"type":             d.Type,
		"mime_type":        d.MimeType,
		"size":             d.Size,
		"extension":        d.Extension,
		"storage_path":     d.StoragePath,
		"etag":             d.ETag,
		"is_starred":       d.IsStarred,
		"status":           d.Status,
		"created_at":       d.CreatedAt,
		"updated_at":       d.UpdatedAt,
	}
}
