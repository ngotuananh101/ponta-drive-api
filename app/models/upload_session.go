package models

import (
	"encoding/json"

	"github.com/goravel/framework/database/orm"
	"github.com/goravel/framework/support/carbon"

	"ponta_drive/app/contracts"
)

// UploadSession tracks an in-progress multipart or chunked upload (spec 2.3).
type UploadSession struct {
	orm.Model
	SessionID      string           `gorm:"column:session_id;size:36;uniqueIndex;not null" json:"session_id"`
	UploadID       string           `gorm:"column:upload_id;size:255;not null" json:"upload_id"`
	UserID         uint             `gorm:"column:user_id;index;not null" json:"user_id"`
	CloudAccountID uint             `gorm:"column:cloud_account_id;index;not null" json:"cloud_account_id"`
	ParentID       *uint            `gorm:"column:parent_id" json:"parent_id"`
	FileName       string           `gorm:"column:file_name;size:255;not null" json:"file_name"`
	MimeType       string           `gorm:"column:mime_type;size:100" json:"mime_type"`
	TotalSize      int64            `gorm:"column:total_size;not null" json:"total_size"`
	ChunkSize      int64            `gorm:"column:chunk_size;not null" json:"chunk_size"`
	TotalParts     int32            `gorm:"column:total_parts;not null" json:"total_parts"`
	UploadedParts  string           `gorm:"column:uploaded_parts;type:text;not null" json:"uploaded_parts"`
	StoragePath    string           `gorm:"column:storage_path;type:text;not null" json:"storage_path"`
	ExpiresAt      *carbon.DateTime `gorm:"column:expires_at;index;not null" json:"expires_at"`
}

// GetUploadedParts deserializes the JSON string into []contracts.CompletedPart.
func (u *UploadSession) GetUploadedParts() ([]contracts.CompletedPart, error) {
	if u.UploadedParts == "" || u.UploadedParts == "[]" {
		return []contracts.CompletedPart{}, nil
	}
	var parts []contracts.CompletedPart
	if err := json.Unmarshal([]byte(u.UploadedParts), &parts); err != nil {
		return nil, err
	}
	return parts, nil
}

// SetUploadedParts serializes []contracts.CompletedPart into JSON.
func (u *UploadSession) SetUploadedParts(parts []contracts.CompletedPart) error {
	if parts == nil {
		parts = []contracts.CompletedPart{}
	}
	bytes, err := json.Marshal(parts)
	if err != nil {
		return err
	}
	u.UploadedParts = string(bytes)
	return nil
}

// AddUploadedPart appends or updates a CompletedPart entry.
func (u *UploadSession) AddUploadedPart(newPart contracts.CompletedPart) error {
	parts, err := u.GetUploadedParts()
	if err != nil {
		parts = []contracts.CompletedPart{}
	}

	// Update existing part number if already present, otherwise append
	updated := false
	for i, p := range parts {
		if p.PartNumber == newPart.PartNumber {
			parts[i] = newPart
			updated = true
			break
		}
	}
	if !updated {
		parts = append(parts, newPart)
	}

	return u.SetUploadedParts(parts)
}
