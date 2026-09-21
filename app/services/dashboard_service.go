package services

import (
	"context"
	"fmt"
	"time"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
)

// DashboardService aggregates the data required to render the home dashboard in
// a single call (spec: home dashboard integration).
type DashboardService struct {
	activityService *ActivityService
}

func NewDashboardService() *DashboardService {
	return &DashboardService{
		activityService: NewActivityService(),
	}
}

// DashboardSummary is the aggregate payload returned to the home dashboard.
type DashboardSummary struct {
	Clouds           []map[string]any `json:"clouds"`
	SuggestedFiles   []map[string]any `json:"suggested_files"`
	RecentActivities []map[string]any `json:"recent_activities"`
	TotalStorage     StorageSummary   `json:"total_storage"`
}

// StorageSummary holds aggregate storage figures across all of a user's clouds.
type StorageSummary struct {
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	UsedHuman  string  `json:"used_human"`
	TotalHuman string  `json:"total_human"`
	Percent    float64 `json:"percent"`
}

// GetSummary builds the dashboard payload: connected clouds, suggested files,
// recent activity and the aggregated storage summary.
func (s *DashboardService) GetSummary(ctx context.Context, userID uint) (*DashboardSummary, error) {
	var accounts []models.CloudAccount
	_ = facades.Orm().Query().
		Where("user_id", userID).
		Order("is_default desc, id desc").
		Get(&accounts)

	clouds := make([]map[string]any, 0, len(accounts))
	var totalUsed, totalCapacity int64

	for _, acc := range accounts {
		clouds = append(clouds, acc.ToResponse())
		totalUsed += acc.UsedStorage
		totalCapacity += acc.TotalStorage
	}

	suggestedFiles, _ := s.getSuggestedFiles(ctx, userID)

	activities, _ := s.activityService.GetRecent(userID, 8)
	actData := make([]map[string]any, 0, len(activities))
	for _, a := range activities {
		actData = append(actData, a.ToResponse())
	}

	percent := 0.0
	if totalCapacity > 0 {
		percent = float64(totalUsed) / float64(totalCapacity) * 100
	}

	return &DashboardSummary{
		Clouds:           clouds,
		SuggestedFiles:   suggestedFiles,
		RecentActivities: actData,
		TotalStorage: StorageSummary{
			UsedBytes:  totalUsed,
			TotalBytes: totalCapacity,
			UsedHuman:  humanizeBytes(totalUsed),
			TotalHuman: humanizeBytes(totalCapacity),
			Percent:    percent,
		},
	}, nil
}

// getSuggestedFiles prefers starred items, then tops up with files updated in
// the last seven days, capped at 8 entries.
func (s *DashboardService) getSuggestedFiles(ctx context.Context, userID uint) ([]map[string]any, error) {
	var items []models.DriveItem

	_ = facades.Orm().Query().
		Where("user_id", userID).
		Where("is_starred", true).
		Where("status", models.ItemStatusReady).
		Order("updated_at desc").
		Limit(4).
		Get(&items)

	if len(items) < 8 {
		var recent []models.DriveItem
		sevenDaysAgo := time.Now().AddDate(0, 0, -7)
		_ = facades.Orm().Query().
			Where("user_id", userID).
			Where("is_starred", false).
			Where("status", models.ItemStatusReady).
			Where("updated_at > ?", sevenDaysAgo).
			Order("updated_at desc").
			Limit(8 - len(items)).
			Get(&recent)
		items = append(items, recent...)
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, item.ToResponse())
	}
	return result, nil
}

// humanizeBytes formats a byte count into a human-readable string using binary
// (1024-based) units, e.g. 1536 -> "1.5 KB".
func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
