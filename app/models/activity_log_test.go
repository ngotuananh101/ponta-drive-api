package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestActivityLogToResponse(t *testing.T) {
	targetUUID := "3f2b6d0e-1a4c-4f2e-9b7a-8c1d2e3f4a5b"
	cloudAccountID := uint(7)
	createdAt := time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC)

	log := ActivityLog{
		ID:             42,
		UserID:         1,
		Action:         "upload",
		TargetName:     "holiday-photo.jpg",
		TargetUUID:     &targetUUID,
		CloudAccountID: &cloudAccountID,
		IPAddress:      "203.0.113.10",
		UserAgent:      "Mozilla/5.0 (Test)",
		Metadata:       `{"size":1024}`,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}

	resp := log.ToResponse()

	assert.Equal(t, uint(42), resp["id"])
	assert.Equal(t, "upload", resp["action"])
	assert.Equal(t, "holiday-photo.jpg", resp["target_name"])
	assert.Equal(t, &targetUUID, resp["target_uuid"])
	assert.Equal(t, &cloudAccountID, resp["cloud_id"])
	assert.Equal(t, "203.0.113.10", resp["ip_address"])
	assert.Equal(t, "Mozilla/5.0 (Test)", resp["user_agent"])

	created, ok := resp["created_at"].(string)
	assert.True(t, ok, "created_at should be a string")
	assert.True(t, strings.Contains(created, "20"), "created_at should contain the year prefix")
	assert.Equal(t, createdAt.Format(time.RFC3339), created)
}

func TestActivityLogTableName(t *testing.T) {
	assert.Equal(t, "activity_logs", ActivityLog{}.TableName())
}
