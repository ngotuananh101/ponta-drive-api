package models

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/goravel/framework/database/orm"

	"ponta_drive/app/facades"
)

// Supported cloud storage provider identifiers
const (
	ProviderS3           = "s3"
	ProviderCloudflareR2 = "cloudflare_r2"
	ProviderMinIO        = "minio"
	ProviderGoogleDrive  = "google_drive"
	ProviderOneDrive     = "onedrive"
)

// S3Credentials contains the connection parameters for S3 and S3-compatible providers.
type S3Credentials struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	UsePathStyle    bool   `json:"use_path_style"`
	PublicURL       string `json:"public_url,omitempty"`
}

// CloudAccount manages personal cloud storage connections of each user (spec 2.1).
type CloudAccount struct {
	orm.Model
	UserID      uint   `gorm:"column:user_id;index;not null" json:"user_id"`
	Name        string `gorm:"column:name;size:100;not null" json:"name"`
	Provider    string `gorm:"column:provider;size:50;not null;index" json:"provider"`
	Credentials string `gorm:"column:credentials;type:text;not null" json:"-"`
	IsDefault   bool   `gorm:"column:is_default;default:false" json:"is_default"`
	IsActive    bool   `gorm:"column:is_active;default:true" json:"is_active"`
	// SyncStatus tracks the current synchronisation state: idle, syncing or error.
	SyncStatus string `gorm:"column:sync_status;type:enum('idle','syncing','error');default:idle" json:"sync_status"`
	// LastSyncedAt is nil when the account has never been synchronised.
	LastSyncedAt *time.Time `gorm:"column:last_synced_at" json:"last_synced_at"`
	TotalStorage int64      `gorm:"column:total_storage;default:0" json:"total_storage"`
	UsedStorage  int64      `gorm:"column:used_storage;default:0" json:"used_storage"`
	orm.SoftDeletes
}

// SetCredentials serializes and encrypts the given S3Credentials using AES-256 via facades.Crypt().
func (c *CloudAccount) SetCredentials(creds *S3Credentials) error {
	if creds == nil {
		return errors.New("credentials cannot be nil")
	}
	bytes, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	encrypted, err := facades.Crypt().EncryptString(string(bytes))
	if err != nil {
		return err
	}
	c.Credentials = encrypted
	return nil
}

// GetCredentials decrypts and deserializes the stored credentials.
func (c *CloudAccount) GetCredentials() (*S3Credentials, error) {
	if c.Credentials == "" {
		return nil, errors.New("credentials are empty")
	}
	decrypted, err := facades.Crypt().DecryptString(c.Credentials)
	if err != nil {
		return nil, err
	}
	var creds S3Credentials
	if err := json.Unmarshal([]byte(decrypted), &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// ToResponse returns a sanitized map representation for API responses, masking SecretAccessKey.
func (c *CloudAccount) ToResponse() map[string]any {
	var safeCreds map[string]any
	if creds, err := c.GetCredentials(); err == nil && creds != nil {
		safeCreds = map[string]any{
			"endpoint":       creds.Endpoint,
			"bucket":         creds.Bucket,
			"region":         creds.Region,
			"access_key_id":  creds.AccessKeyID,
			"use_path_style": creds.UsePathStyle,
			"public_url":     creds.PublicURL,
		}
	}

	var lastSynced any = nil
	if c.LastSyncedAt != nil {
		lastSynced = c.LastSyncedAt.Format(time.RFC3339)
	}

	return map[string]any{
		"id":             c.ID,
		"user_id":        c.UserID,
		"name":           c.Name,
		"provider":       c.Provider,
		"credentials":    safeCreds,
		"is_default":     c.IsDefault,
		"is_active":      c.IsActive,
		"sync_status":    c.SyncStatus,
		"last_synced_at": lastSynced,
		"total_storage":  c.TotalStorage,
		"used_storage":   c.UsedStorage,
		"created_at":     c.CreatedAt,
		"updated_at":     c.UpdatedAt,
	}
}
