package jobs

import (
	"context"
	"fmt"
	"time"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
)

type SyncS3BucketJob struct{}

func (j *SyncS3BucketJob) Signature() string {
	return "sync_s3_bucket_job"
}

func (j *SyncS3BucketJob) Handle(args ...any) error {
	if len(args) < 2 {
		return fmt.Errorf("insufficient arguments: expected userID and cloudAccountID")
	}

	var userID uint
	switch v := args[0].(type) {
	case uint:
		userID = v
	case int:
		userID = uint(v)
	case int64:
		userID = uint(v)
	case float64:
		userID = uint(v)
	}

	var cloudAccountID uint
	switch v := args[1].(type) {
	case uint:
		cloudAccountID = v
	case int:
		cloudAccountID = uint(v)
	case int64:
		cloudAccountID = uint(v)
	case float64:
		cloudAccountID = uint(v)
	}

	var parentID *uint
	if len(args) >= 3 && args[2] != nil {
		switch v := args[2].(type) {
		case uint:
			if v > 0 {
				parentID = &v
			}
		case *uint:
			parentID = v
		case float64:
			if v > 0 {
				u := uint(v)
				parentID = &u
			}
		}
	}

	var itemsCount int
	var scanErr error

	// Always release the account from the "syncing" state, even when the scan
	// fails, otherwise the account stays stuck at "syncing" forever.
	defer func() {
		updates := map[string]any{"sync_status": "idle"}
		if scanErr != nil {
			updates["sync_status"] = "error"
		} else {
			updates["last_synced_at"] = time.Now()
		}
		_, _ = facades.Orm().Query().Where("id", cloudAccountID).Update(updates)

		if scanErr != nil {
			return
		}

		var account models.CloudAccount
		_ = facades.Orm().Query().Where("id", cloudAccountID).First(&account)

		activityService := services.NewActivityService()
		_ = activityService.Log(userID, cloudAccountID, "synced", account.Name, nil, "system", "Queue Worker", map[string]any{"items_count": itemsCount})
	}()

	service := services.NewCloudDriveService()
	itemsCount, scanErr = service.ScanBucket(context.Background(), userID, cloudAccountID, parentID)

	return scanErr
}
