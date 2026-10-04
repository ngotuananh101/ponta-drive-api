package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ponta_drive/app/models"
	_ "ponta_drive/tests"
)

func TestDriveItemToResponseExposesUUIDsNotIDs(t *testing.T) {
	item := models.DriveItem{
		UUID:             "22222222-2222-2222-2222-222222222222",
		UserID:           7,
		CloudAccountID:   3,
		Name:             "report.pdf",
		Type:             models.ItemTypeFile,
		Status:           models.ItemStatusReady,
		CloudAccountUUID: "11111111-1111-1111-1111-111111111111",
	}

	resp := item.ToResponse()

	assert.Equal(t, "22222222-2222-2222-2222-222222222222", resp["uuid"])
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resp["cloud_account_uuid"])
	assert.NotContains(t, resp, "id")
	assert.NotContains(t, resp, "user_id")
	assert.NotContains(t, resp, "cloud_account_id")
	assert.NotContains(t, resp, "parent_id")
}
