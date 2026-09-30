package services

import (
	"context"
	"encoding/json"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
)

// ActivityService records and queries the user activity audit trail surfaced on
// the home dashboard (spec: home dashboard integration).
type ActivityService struct{}

func NewActivityService() *ActivityService {
	return &ActivityService{}
}

// Log persists a single activity log entry. metadata is serialized to JSON;
// a nil map is stored as an empty string.
func (s *ActivityService) Log(
	userID, cloudAccountID uint,
	action, targetName string,
	targetUUID *string,
	ipAddress, userAgent string,
	metadata map[string]any,
) error {
	var metaJSON *string
	if metadata != nil {
		bytes, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		encoded := string(bytes)
		metaJSON = &encoded
	}

	log := models.ActivityLog{
		UserID:         userID,
		Action:         action,
		TargetName:     targetName,
		TargetUUID:     targetUUID,
		CloudAccountID: &cloudAccountID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Metadata:       metaJSON,
	}

	return facades.Orm().Query().Create(&log)
}

// LogSafe records an activity without propagating failures.
//
// Audit logging is best-effort: a failure must never break the user-facing
// operation that triggered it. Failures are reported to the application log so
// gaps in the audit trail remain visible, instead of being silently discarded.
func (s *ActivityService) LogSafe(
	userID, cloudAccountID uint,
	action, targetName string,
	targetUUID *string,
	ipAddress, userAgent string,
	metadata map[string]any,
) {
	if err := s.Log(userID, cloudAccountID, action, targetName, targetUUID, ipAddress, userAgent, metadata); err != nil {
		facades.Log().Errorf(
			"[Activity] Failed to record %q for user %d (target %q): %v",
			action, userID, targetName, err,
		)
	}
}

// GetRecent returns the most recent activities for a user, newest first.
//
// The `id` tiebreaker matters: `created_at` is a TIMESTAMP with second
// granularity, so rows written in the same second compare equal and their
// order would otherwise be undefined.
func (s *ActivityService) GetRecent(ctx context.Context, userID uint, limit int) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	err := facades.Orm().WithContext(ctx).Query().
		Where("user_id", userID).
		Order("created_at desc").
		Order("id desc").
		Limit(limit).
		Get(&logs)
	return logs, err
}

// GetByIP returns recent activities originating from a specific IP address,
// used for security auditing.
func (s *ActivityService) GetByIP(ctx context.Context, ipAddress string, limit int) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	err := facades.Orm().WithContext(ctx).Query().
		Where("ip_address", ipAddress).
		Order("created_at desc").
		Order("id desc").
		Limit(limit).
		Get(&logs)
	return logs, err
}
