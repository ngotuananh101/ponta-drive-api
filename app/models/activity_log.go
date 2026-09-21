package models

import "time"

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
	Metadata       string    `gorm:"type:json" json:"metadata"`
	CreatedAt      time.Time `gorm:"index:idx_user_created" json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName returns the table name for the ActivityLog model.
func (ActivityLog) TableName() string {
	return "activity_logs"
}

// ToResponse returns a sanitized map representation for API responses.
func (a *ActivityLog) ToResponse() map[string]any {
	return map[string]any{
		"id":          a.ID,
		"action":      a.Action,
		"target_name": a.TargetName,
		"target_uuid": a.TargetUUID,
		"cloud_id":    a.CloudAccountID,
		"ip_address":  a.IPAddress,
		"user_agent":  a.UserAgent,
		"created_at":  a.CreatedAt.Format(time.RFC3339),
	}
}
