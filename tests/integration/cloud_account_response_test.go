package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ponta_drive/app/models"
	_ "ponta_drive/tests"
)

func TestCloudAccountToResponseIncludesSyncStatus(t *testing.T) {
	lastSynced := time.Date(2026, 9, 21, 23, 29, 0, 0, time.UTC)

	account := models.CloudAccount{
		UserID:       10,
		Name:         "Primary S3",
		Provider:     models.ProviderS3,
		IsDefault:    true,
		IsActive:     true,
		SyncStatus:   "idle",
		LastSyncedAt: &lastSynced,
		TotalStorage: 10 * 1024 * 1024 * 1024,
		UsedStorage:  512 * 1024 * 1024,
	}

	creds := &models.S3Credentials{
		Endpoint:        "https://s3.us-east-1.amazonaws.com",
		Bucket:          "ponta-prod-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "super-secret-value",
	}
	require.NoError(t, account.SetCredentials(creds))

	resp := account.ToResponse()

	assert.Equal(t, "idle", resp["sync_status"])
	assert.Equal(t, lastSynced.Format(time.RFC3339), resp["last_synced_at"])
	assert.Equal(t, int64(10*1024*1024*1024), resp["total_storage"])
	assert.Equal(t, int64(512*1024*1024), resp["used_storage"])

	// The secret must never be surfaced in an API response.
	safeCreds, ok := resp["credentials"].(map[string]any)
	require.True(t, ok, "credentials should be a map")
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", safeCreds["access_key_id"])
	assert.NotContains(t, safeCreds, "secret_access_key")
	assert.NotContains(t, resp, "credentials_raw")
}

func TestCloudAccountToResponseWithoutSync(t *testing.T) {
	account := models.CloudAccount{
		UserID:     10,
		Name:       "Never synced",
		Provider:   models.ProviderS3,
		SyncStatus: "idle",
	}
	require.NoError(t, account.SetCredentials(&models.S3Credentials{
		Endpoint: "https://s3.us-east-1.amazonaws.com",
		Bucket:   "bucket",
		Region:   "us-east-1",
	}))

	resp := account.ToResponse()

	assert.Nil(t, resp["last_synced_at"], "an account that never synced should report null")
}

func TestCloudAccountToResponseExposesUUIDNotID(t *testing.T) {
	account := models.CloudAccount{
		UUID:       "11111111-1111-1111-1111-111111111111",
		UserID:     10,
		Name:       "Primary S3",
		Provider:   models.ProviderS3,
		SyncStatus: "idle",
	}

	resp := account.ToResponse()

	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resp["uuid"])
	assert.NotContains(t, resp, "id", "numeric id must not be exposed")
	assert.NotContains(t, resp, "user_id", "user_id must not be exposed")
}

// A row that slipped past the migration/hook (raw SQL insert, or created before
// the column existed) must still serialize a usable uuid rather than a blank
// string, which would break every URL built from it.
func TestCloudAccountToResponseHealsBlankUUID(t *testing.T) {
	account := models.CloudAccount{
		UserID:   10,
		Name:     "Legacy S3",
		Provider: models.ProviderS3,
	}

	resp := account.ToResponse()

	uuid, _ := resp["uuid"].(string)
	assert.Len(t, uuid, 36, "a blank uuid must be replaced on read")
}
