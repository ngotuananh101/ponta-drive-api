package services

import (
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
	metaJSON := ""
	if metadata != nil {
		bytes, _ := json.Marshal(metadata)
		metaJSON = string(bytes)
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

// GetRecent returns the most recent activities for a user, newest first.
func (s *ActivityService) GetRecent(userID uint, limit int) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	err := facades.Orm().Query().
		Where("user_id", userID).
		Order("created_at desc").
		Limit(limit).
		Get(&logs)
	return logs, err
}

// GetByIP returns recent activities originating from a specific IP address,
// used for security auditing.
func (s *ActivityService) GetByIP(ipAddress string, limit int) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	err := facades.Orm().Query().
		Where("ip_address", ipAddress).
		Order("created_at desc").
		Limit(limit).
		Get(&logs)
	return logs, err
}
