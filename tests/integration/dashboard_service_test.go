package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
	_ "ponta_drive/tests"
)

// dashboardFixture is a self-contained user with two clouds and a handful of
// drive items, removed in full on cleanup.
type dashboardFixture struct {
	user     models.User
	cloudA   models.CloudAccount
	cloudB   models.CloudAccount
	starred  models.DriveItem
	recent   models.DriveItem
	starred2 models.DriveItem
}

func seedDashboardFixture(t *testing.T) *dashboardFixture {
	t.Helper()

	f := &dashboardFixture{}

	hashed, err := facades.Hash().Make("password123")
	require.NoError(t, err)

	f.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Dashboard User",
		Username: "dash_" + uuid.New().String()[:8],
		Email:    "dash_" + uuid.New().String()[:8] + "@example.com",
		Password: hashed,
	}
	require.NoError(t, facades.Orm().Query().Create(&f.user))

	f.cloudA = models.CloudAccount{
		UserID:       f.user.ID,
		Name:         "Primary S3",
		Provider:     models.ProviderS3,
		IsDefault:    true,
		IsActive:     true,
		SyncStatus:   "idle",
		TotalStorage: 10 * 1024 * 1024 * 1024, // 10 GB
		UsedStorage:  2 * 1024 * 1024 * 1024,  // 2 GB
	}
	require.NoError(t, f.cloudA.SetCredentials(&models.S3Credentials{
		Endpoint: "https://s3.us-east-1.amazonaws.com",
		Bucket:   "dash-bucket-a",
		Region:   "us-east-1",
	}))
	require.NoError(t, facades.Orm().Query().Create(&f.cloudA))

	f.cloudB = models.CloudAccount{
		UserID:       f.user.ID,
		Name:         "Backup R2",
		Provider:     models.ProviderCloudflareR2,
		IsDefault:    false,
		IsActive:     true,
		SyncStatus:   "idle",
		TotalStorage: 5 * 1024 * 1024 * 1024, // 5 GB
		UsedStorage:  1 * 1024 * 1024 * 1024, // 1 GB
	}
	require.NoError(t, f.cloudB.SetCredentials(&models.S3Credentials{
		Endpoint: "https://acct.r2.cloudflarestorage.com",
		Bucket:   "dash-bucket-b",
		Region:   "auto",
	}))
	require.NoError(t, facades.Orm().Query().Create(&f.cloudB))

	newItem := func(name string, starred bool) models.DriveItem {
		return models.DriveItem{
			UUID:           uuid.New().String(),
			UserID:         f.user.ID,
			CloudAccountID: f.cloudA.ID,
			Name:           name,
			Type:           "file",
			MimeType:       "image/jpeg",
			Size:           2048,
			Status:         models.ItemStatusReady,
			IsStarred:      starred,
		}
	}

	f.starred = newItem("starred-one.jpg", true)
	f.starred2 = newItem("starred-two.jpg", true)
	f.recent = newItem("recent.jpg", false)

	require.NoError(t, facades.Orm().Query().Create(&f.starred))
	require.NoError(t, facades.Orm().Query().Create(&f.starred2))
	require.NoError(t, facades.Orm().Query().Create(&f.recent))

	// An activity so recent_activities is non-empty.
	require.NoError(t, services.NewActivityService().Log(
		f.user.ID, f.cloudA.ID, "uploaded", "recent.jpg", &f.recent.UUID,
		"203.0.113.10", "Test/1.0", map[string]any{"size": 2048}))

	t.Cleanup(func() {
		_, _ = facades.Orm().Query().Where("user_id", f.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", f.user.ID).Delete(&models.ActivityLog{})
		_, _ = facades.Orm().Query().Where("user_id", f.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&f.user)
	})

	return f
}

func TestDashboardServiceGetSummaryAggregatesUserData(t *testing.T) {
	f := seedDashboardFixture(t)
	service := services.NewDashboardService()

	summary, err := service.GetSummary(context.Background(), f.user.ID)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Clouds: both accounts, default first.
	require.Len(t, summary.Clouds, 2)
	assert.Equal(t, "Primary S3", summary.Clouds[0]["name"])

	// Storage aggregates across both clouds: 3 GB used of 15 GB.
	assert.Equal(t, int64(3*1024*1024*1024), summary.TotalStorage.UsedBytes)
	assert.Equal(t, int64(15*1024*1024*1024), summary.TotalStorage.TotalBytes)
	assert.Equal(t, "3.0 GB", summary.TotalStorage.UsedHuman)
	assert.Equal(t, "15.0 GB", summary.TotalStorage.TotalHuman)
	assert.InDelta(t, 20.0, summary.TotalStorage.Percent, 0.01)

	// Activities include the seeded upload, with metadata decoded.
	require.NotEmpty(t, summary.RecentActivities)
	assert.Equal(t, "uploaded", summary.RecentActivities[0]["action"])
	assert.Equal(t, map[string]any{"size": float64(2048)}, summary.RecentActivities[0]["metadata"])
}

func TestDashboardServiceGetSummaryPrefersStarredFiles(t *testing.T) {
	f := seedDashboardFixture(t)
	service := services.NewDashboardService()

	summary, err := service.GetSummary(context.Background(), f.user.ID)
	require.NoError(t, err)

	require.Len(t, summary.SuggestedFiles, 3)

	// Starred items are surfaced before non-starred ones. All three were created
	// within the same second, so this also pins the `id` tiebreaker: the
	// later-inserted starred item must rank first.
	assert.Equal(t, true, summary.SuggestedFiles[0]["is_starred"])
	assert.Equal(t, true, summary.SuggestedFiles[1]["is_starred"])
	assert.Equal(t, false, summary.SuggestedFiles[2]["is_starred"])
	assert.Equal(t, "starred-two.jpg", summary.SuggestedFiles[0]["name"])
	assert.Equal(t, "starred-one.jpg", summary.SuggestedFiles[1]["name"])
	assert.Equal(t, "recent.jpg", summary.SuggestedFiles[2]["name"])
}

func TestDashboardServiceGetSummaryPropagatesQueryErrors(t *testing.T) {
	f := seedDashboardFixture(t)

	// A cancelled context makes the underlying database/sql query fail. If
	// GetSummary swallows that failure the caller gets an empty dashboard and a
	// nil error, which makes the controller's 500 branch unreachable and hides
	// the outage entirely.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	summary, err := services.NewDashboardService().GetSummary(ctx, f.user.ID)

	require.Error(t, err, "a failing query must surface as an error, not a silently empty dashboard")
	assert.Nil(t, summary)
}

func TestDashboardServiceGetSummaryIsScopedToUser(t *testing.T) {
	seedDashboardFixture(t)
	service := services.NewDashboardService()

	// A user with no data must get empty collections, not another user's rows.
	summary, err := service.GetSummary(context.Background(), 987654322)
	require.NoError(t, err)

	assert.Empty(t, summary.Clouds)
	assert.Empty(t, summary.SuggestedFiles)
	assert.Empty(t, summary.RecentActivities)
	assert.Equal(t, int64(0), summary.TotalStorage.TotalBytes)
	assert.Equal(t, 0.0, summary.TotalStorage.Percent)
}

func TestDashboardServiceGetSummaryExcludesStaleFiles(t *testing.T) {
	f := seedDashboardFixture(t)

	// A file that is neither starred nor updated in the last seven days must not
	// be suggested.
	stale := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         f.user.ID,
		CloudAccountID: f.cloudA.ID,
		Name:           "ancient.jpg",
		Type:           "file",
		MimeType:       "image/jpeg",
		Size:           10,
		Status:         models.ItemStatusReady,
		IsStarred:      false,
	}
	require.NoError(t, facades.Orm().Query().Create(&stale))
	_, err := facades.Orm().Query().Model(&models.DriveItem{}).
		Where("uuid", stale.UUID).
		Update(map[string]any{"updated_at": time.Now().AddDate(0, 0, -30)})
	require.NoError(t, err)

	summary, err := services.NewDashboardService().GetSummary(context.Background(), f.user.ID)
	require.NoError(t, err)

	for _, file := range summary.SuggestedFiles {
		assert.NotEqual(t, "ancient.jpg", file["name"])
	}
}
