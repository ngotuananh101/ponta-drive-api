package models

import (
	"encoding/json"
	"time"
)

// ActivityLog records user actions as an audit trail surfaced on the home
// dashboard (spec: home dashboard integration).
//
// Notes:
//   - `target_uuid` and `cloud_account_id` are nullable because not every
//     action targets a UUID-keyed resource or a cloud account.
//   - `metadata` holds an action-specific JSON payload serialized as a string.
type ActivityLog struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;index" json:"user_id"`
	Action         string    `gorm:"size:50;not null" json:"action"`
	TargetName     string    `gorm:"size:255;not null" json:"target_name"`
	TargetUUID     *string   `gorm:"size:36" json:"target_uuid"`
	CloudAccountID *uint     `json:"cloud_account_id"`
	IPAddress      string    `gorm:"size:45;index" json:"ip_address"`
	UserAgent      string    `gorm:"type:text" json:"user_agent"`
	Metadata       *string   `gorm:"type:json" json:"metadata"`
	CreatedAt      time.Time `gorm:"index:idx_user_created" json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName returns the table name for the ActivityLog model.
func (ActivityLog) TableName() string {
	return "activity_logs"
}

// ToResponse returns a sanitized map representation for API responses.
//
// `metadata` is decoded from its stored JSON string so consumers receive a
// structured object (e.g. `{"items_count": 12}`) rather than an escaped string.
// A nil or non-decodable value is surfaced as nil.
func (a *ActivityLog) ToResponse() map[string]any {
	return map[string]any{
		"id":               a.ID,
		"action":           a.Action,
		"target_name":      a.TargetName,
		"target_uuid":      a.TargetUUID,
		"cloud_account_id": a.CloudAccountID,
		"ip_address":       a.IPAddress,
		"user_agent":       a.UserAgent,
		"metadata":         a.decodedMetadata(),
		"created_at":       a.CreatedAt.Format(time.RFC3339),
	}
}

// decodedMetadata unmarshals the stored metadata JSON into a generic map.
func (a *ActivityLog) decodedMetadata() any {
	if a.Metadata == nil || *a.Metadata == "" {
		return nil
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(*a.Metadata), &decoded); err != nil {
		return nil
	}

	return decoded
}
