package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ponta_drive/app/models"
	_ "ponta_drive/tests" // Initialize test environment and facades.
)

func TestDriveItemToResponseExposesUUIDAndCloudAccountID(t *testing.T) {
	item := models.DriveItem{
		UUID:           "22222222-2222-2222-2222-222222222222",
		UserID:         7,
		CloudAccountID: 3,
		Name:           "report.pdf",
		Type:           models.ItemTypeFile,
		Status:         models.ItemStatusReady,
	}

	resp := item.ToResponse()

	assert.Equal(t, "22222222-2222-2222-2222-222222222222", resp["uuid"])
	assert.Equal(t, uint(3), resp["cloud_account_id"])
	assert.NotContains(t, resp, "id")
	assert.NotContains(t, resp, "user_id")
	assert.NotContains(t, resp, "parent_id")
}
