package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
	_ "ponta_drive/tests"
)

// activityUserID is an arbitrary id used to scope the rows created by these
// tests so they can be cleaned up without touching other data.
const activityUserID uint = 987654321

func cleanupActivities(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = facades.Orm().Query().Where("user_id", activityUserID).Delete(&models.ActivityLog{})
	})
}

func TestActivityServiceLogPersistsEntry(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	targetUUID := "3f2b6d0e-1a4c-4f2e-9b7a-8c1d2e3f4a5b"
	err := service.Log(activityUserID, 42, "uploaded", "holiday.jpg", &targetUUID,
		"203.0.113.10", "Mozilla/5.0 (Test)", map[string]any{"size": 1024, "mime": "image/jpeg"})
	require.NoError(t, err)

	logs, err := service.GetRecent(context.Background(), activityUserID, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	assert.Equal(t, "uploaded", logs[0].Action)
	assert.Equal(t, "holiday.jpg", logs[0].TargetName)
	assert.Equal(t, "203.0.113.10", logs[0].IPAddress)
	assert.Equal(t, "Mozilla/5.0 (Test)", logs[0].UserAgent)
	require.NotNil(t, logs[0].Metadata)
	assert.Equal(t, `{"mime":"image/jpeg","size":1024}`, *logs[0].Metadata)
}

func TestActivityServiceLogWithNilMetadata(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	// Several callers (folder creation, rename, sync_started) log without
	// metadata. The column is a nullable JSON column, so a nil map must persist
	// successfully rather than failing JSON validation.
	err := service.Log(activityUserID, 42, "created_folder", "Photos", nil,
		"203.0.113.10", "Mozilla/5.0 (Test)", nil)
	require.NoError(t, err)

	logs, err := service.GetRecent(context.Background(), activityUserID, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "created_folder", logs[0].Action)
	assert.Nil(t, logs[0].Metadata, "nil metadata should persist as SQL NULL")
}

func TestActivityServiceGetRecentOrdersNewestFirstAndLimits(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	for _, action := range []string{"first", "second", "third"} {
		require.NoError(t, service.Log(activityUserID, 1, action, action+".txt", nil,
			"198.51.100.1", "Test/1.0", nil))
	}

	logs, err := service.GetRecent(context.Background(), activityUserID, 2)
	require.NoError(t, err)
	require.Len(t, logs, 2)

	// `created_at` is second-granular, so all three rows typically share a
	// timestamp. Asserting on time alone would pass vacuously; the `id`
	// tiebreaker is what makes "newest first" deterministic.
	assert.Equal(t, "third", logs[0].Action, "newest activity must come first")
	assert.Equal(t, "second", logs[1].Action, "limit must drop the oldest row")
	assert.Greater(t, logs[0].ID, logs[1].ID, "ties are broken by descending id")
}

func TestActivityServiceGetByIPScopesToAddress(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	require.NoError(t, service.Log(activityUserID, 1, "from_a", "a.txt", nil,
		"10.0.0.1", "Test/1.0", nil))
	require.NoError(t, service.Log(activityUserID, 1, "from_b", "b.txt", nil,
		"10.0.0.2", "Test/1.0", nil))

	logs, err := service.GetByIP(context.Background(), "10.0.0.1", 10)
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	for _, l := range logs {
		assert.Equal(t, "10.0.0.1", l.IPAddress)
	}
}

func TestActivityServiceLogReturnsMarshalError(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	// Metadata that cannot be marshalled must surface as an error rather than
	// silently writing an invalid JSON value.
	err := service.Log(activityUserID, 7, "uploaded", "bad.bin", nil,
		"203.0.113.10", "Test/1.0", map[string]any{"bad": make(chan int)})

	require.Error(t, err)
}

func TestActivityServiceLogSafePersistsEntry(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	// LogSafe is the fire-and-forget variant used by the controllers: it must
	// still persist the entry.
	service.LogSafe(activityUserID, 7, "created_folder", "Photos", nil,
		"203.0.113.10", "Mozilla/5.0 (Test)", nil)

	logs, err := service.GetRecent(context.Background(), activityUserID, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "created_folder", logs[0].Action)
	assert.Equal(t, "Photos", logs[0].TargetName)
}

func TestActivityServiceLogSafeSwallowsFailure(t *testing.T) {
	cleanupActivities(t)
	service := services.NewActivityService()

	// Audit logging must never break the user-facing operation: a failure is
	// reported to the app log but not propagated, and leaves no partial row.
	service.LogSafe(activityUserID, 7, "uploaded", "bad.bin", nil,
		"203.0.113.10", "Test/1.0", map[string]any{"bad": make(chan int)})

	logs, err := service.GetRecent(context.Background(), activityUserID, 10)
	require.NoError(t, err)
	assert.Empty(t, logs, "a failed log must not leave a partial row")
}
