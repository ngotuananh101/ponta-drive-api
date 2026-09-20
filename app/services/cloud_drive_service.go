package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/goravel/framework/support/carbon"

	"ponta_drive/app/contracts"
	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services/storage"
)

// CloudDriveService orchestrates file hierarchy management and cloud storage operations.
type CloudDriveService struct {
	factory *storage.DriverFactory
}

func NewCloudDriveService() *CloudDriveService {
	return &CloudDriveService{
		factory: storage.NewDriverFactory(),
	}
}

// GetDriver resolves a CloudDriver and verifies ownership of the CloudAccount.
func (s *CloudDriveService) GetDriver(ctx context.Context, userID uint, cloudAccountID uint) (contracts.CloudDriver, *models.CloudAccount, error) {
	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("id", cloudAccountID).
		Where("user_id", userID).
		First(&account)
	if err != nil || account.ID == 0 {
		return nil, nil, errors.New("cloud account not found or access denied")
	}

	driver, err := s.factory.DriverForAccount(ctx, &account)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize driver: %w", err)
	}

	return driver, &account, nil
}

// CreateFolder creates a virtual folder under the specified parent directory.
func (s *CloudDriveService) CreateFolder(ctx context.Context, userID uint, cloudAccountID uint, parentID *uint, name string) (*models.DriveItem, error) {
	// Verify account belongs to user
	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("id", cloudAccountID).
		Where("user_id", userID).
		First(&account)
	if err != nil || account.ID == 0 {
		return nil, errors.New("invalid cloud account")
	}

	// If parentID provided, verify it exists and is a folder
	if parentID != nil && *parentID > 0 {
		var parent models.DriveItem
		err := facades.Orm().Query().
			Where("id", *parentID).
			Where("user_id", userID).
			Where("cloud_account_id", cloudAccountID).
			First(&parent)
		if err != nil || parent.ID == 0 {
			return nil, errors.New("parent folder not found")
		}
		if !parent.IsFolder() {
			return nil, errors.New("parent item is not a folder")
		}
	}

	folder := &models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         userID,
		CloudAccountID: cloudAccountID,
		ParentID:       parentID,
		Name:           strings.TrimSpace(name),
		Type:           models.ItemTypeFolder,
		Status:         models.ItemStatusReady,
	}

	if err := facades.Orm().Query().Create(folder); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return folder, nil
}

// ListItems queries items for a given cloud account and parent directory.
func (s *CloudDriveService) ListItems(ctx context.Context, userID uint, cloudAccountID uint, parentID *uint, search string, itemType string, sortField string, sortOrder string) ([]models.DriveItem, error) {
	query := facades.Orm().Query().
		Where("user_id", userID).
		Where("cloud_account_id", cloudAccountID)

	if parentID == nil || *parentID == 0 {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id", *parentID)
	}

	if strings.TrimSpace(search) != "" {
		query = query.Where("name LIKE ?", "%"+strings.TrimSpace(search)+"%")
	}

	if itemType == models.ItemTypeFolder || itemType == models.ItemTypeFile {
		query = query.Where("type", itemType)
	}

	// Ordering
	orderCol := "name"
	switch sortField {
	case "name", "size", "updated_at", "created_at":
		orderCol = sortField
	}

	orderDir := "asc"
	if strings.ToLower(sortOrder) == "desc" {
		orderDir = "desc"
	}

	// Folders first, then orderCol
	query = query.Order(fmt.Sprintf("CASE WHEN type = '%s' THEN 0 ELSE 1 END, %s %s", models.ItemTypeFolder, orderCol, orderDir))

	var items []models.DriveItem
	if err := query.Get(&items); err != nil {
		return nil, err
	}

	return items, nil
}

// GetItemByUUID returns an item by public UUID.
func (s *CloudDriveService) GetItemByUUID(ctx context.Context, userID uint, itemUUID string) (*models.DriveItem, error) {
	var item models.DriveItem
	err := facades.Orm().Query().
		Where("uuid", itemUUID).
		Where("user_id", userID).
		First(&item)
	if err != nil || item.ID == 0 {
		return nil, errors.New("item not found")
	}
	return &item, nil
}

// UpdateItem updates name or parent folder of an item.
func (s *CloudDriveService) UpdateItem(ctx context.Context, userID uint, itemUUID string, newName string, newParentID *uint) (*models.DriveItem, error) {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(newName) != "" {
		item.Name = strings.TrimSpace(newName)
	}

	if newParentID != nil {
		if *newParentID == item.ID {
			return nil, errors.New("an item cannot be its own parent")
		}
		if *newParentID > 0 {
			var parent models.DriveItem
			err := facades.Orm().Query().
				Where("id", *newParentID).
				Where("user_id", userID).
				First(&parent)
			if err != nil || parent.ID == 0 {
				return nil, errors.New("destination folder not found")
			}
			if !parent.IsFolder() {
				return nil, errors.New("destination is not a folder")
			}
			item.ParentID = newParentID
		} else {
			item.ParentID = nil
		}
	}

	if err := facades.Orm().Query().Save(item); err != nil {
		return nil, err
	}

	return item, nil
}

// ToggleStar toggles the starred status of an item.
func (s *CloudDriveService) ToggleStar(ctx context.Context, userID uint, itemUUID string) (*models.DriveItem, error) {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return nil, err
	}

	item.IsStarred = !item.IsStarred
	if err := facades.Orm().Query().Save(item); err != nil {
		return nil, err
	}

	return item, nil
}

// DeleteItem deletes an item (soft-delete or permanent delete).
func (s *CloudDriveService) DeleteItem(ctx context.Context, userID uint, itemUUID string, permanent bool) error {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return err
	}

	// If permanent and it is a file on S3, delete object from bucket
	if permanent && item.IsFile() && item.StoragePath != "" {
		driver, _, err := s.GetDriver(ctx, userID, item.CloudAccountID)
		if err == nil && driver != nil {
			_ = driver.Delete(ctx, item.StoragePath)
		}
	}

	if permanent {
		_, err = facades.Orm().Query().ForceDelete(item)
	} else {
		_, err = facades.Orm().Query().Delete(item)
	}

	return err
}

// InitiatePresignedUpload creates a pending DriveItem and generates an S3 Presigned Upload URL.
func (s *CloudDriveService) InitiatePresignedUpload(ctx context.Context, userID uint, cloudAccountID uint, parentID *uint, fileName string, size int64, mimeType string) (*models.DriveItem, *contracts.PresignedURLResponse, error) {
	driver, _, err := s.GetDriver(ctx, userID, cloudAccountID)
	if err != nil {
		return nil, nil, err
	}

	if parentID != nil && *parentID > 0 {
		var parent models.DriveItem
		err := facades.Orm().Query().
			Where("id", *parentID).
			Where("user_id", userID).
			Where("cloud_account_id", cloudAccountID).
			First(&parent)
		if err != nil || parent.ID == 0 || !parent.IsFolder() {
			return nil, nil, errors.New("parent folder not found or is not a folder")
		}
	}

	fileExt := strings.TrimPrefix(filepath.Ext(fileName), ".")
	storageKey := fmt.Sprintf("users/%d/items/%s-%s", userID, uuid.New().String(), fileName)

	item := &models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         userID,
		CloudAccountID: cloudAccountID,
		ParentID:       parentID,
		Name:           fileName,
		Type:           models.ItemTypeFile,
		MimeType:       mimeType,
		Size:           size,
		Extension:      fileExt,
		StoragePath:    storageKey,
		Status:         models.ItemStatusUploading,
	}

	if err := facades.Orm().Query().Create(item); err != nil {
		return nil, nil, fmt.Errorf("failed to create pending drive item: %w", err)
	}

	presignedResp, err := driver.GetPresignedUploadURL(ctx, storageKey, mimeType, 15*time.Minute)
	if err != nil {
		_, _ = facades.Orm().Query().ForceDelete(item)
		return nil, nil, fmt.Errorf("failed to generate presigned upload url: %w", err)
	}

	return item, presignedResp, nil
}

// CompletePresignedUpload verifies the uploaded object on S3 and marks the DriveItem as ready.
func (s *CloudDriveService) CompletePresignedUpload(ctx context.Context, userID uint, itemUUID string) (*models.DriveItem, error) {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return nil, err
	}

	driver, account, err := s.GetDriver(ctx, userID, item.CloudAccountID)
	if err != nil {
		return nil, err
	}

	meta, err := driver.GetMetadata(ctx, item.StoragePath)
	if err != nil {
		item.Status = models.ItemStatusFailed
		_ = facades.Orm().Query().Save(item)
		return nil, fmt.Errorf("uploaded file not found on storage: %w", err)
	}

	item.Status = models.ItemStatusReady
	item.ETag = meta.ETag
	if meta.Size > 0 {
		item.Size = meta.Size
	}
	if meta.MimeType != "" {
		item.MimeType = meta.MimeType
	}

	if err := facades.Orm().Query().Save(item); err != nil {
		return nil, fmt.Errorf("failed to finalize drive item: %w", err)
	}

	// Increment used storage on cloud account
	if account != nil && item.Size > 0 {
		account.UsedStorage += item.Size
		_ = facades.Orm().Query().Save(account)
	}

	return item, nil
}

// InitiateMultipartUpload starts an S3 multipart upload session and stores tracking metadata.
func (s *CloudDriveService) InitiateMultipartUpload(ctx context.Context, userID uint, cloudAccountID uint, parentID *uint, fileName string, size int64, mimeType string, chunkSize int64) (*models.UploadSession, error) {
	driver, _, err := s.GetDriver(ctx, userID, cloudAccountID)
	if err != nil {
		return nil, err
	}

	if parentID != nil && *parentID > 0 {
		var parent models.DriveItem
		err := facades.Orm().Query().
			Where("id", *parentID).
			Where("user_id", userID).
			Where("cloud_account_id", cloudAccountID).
			First(&parent)
		if err != nil || parent.ID == 0 {
			return nil, errors.New("parent folder not found")
		}
	}

	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024 // 5MB default chunk size
	}

	totalParts := int32((size + chunkSize - 1) / chunkSize)
	if totalParts < 1 {
		totalParts = 1
	}

	storageKey := fmt.Sprintf("users/%d/items/%s-%s", userID, uuid.New().String(), fileName)

	uploadID, err := driver.InitiateMultipart(ctx, storageKey, mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate multipart upload on storage: %w", err)
	}

	now := carbon.Now()
	exp := now.AddDays(1)
	expDateTime := carbon.NewDateTime(exp)

	session := &models.UploadSession{
		SessionID:      uuid.New().String(),
		UploadID:       uploadID,
		UserID:         userID,
		CloudAccountID: cloudAccountID,
		ParentID:       parentID,
		FileName:       fileName,
		MimeType:       mimeType,
		TotalSize:      size,
		ChunkSize:      chunkSize,
		TotalParts:     totalParts,
		UploadedParts:  "[]",
		StoragePath:    storageKey,
		ExpiresAt:      expDateTime,
	}

	if err := facades.Orm().Query().Create(session); err != nil {
		_ = driver.AbortMultipart(ctx, storageKey, uploadID)
		return nil, fmt.Errorf("failed to save upload session: %w", err)
	}

	return session, nil
}

// UploadPart streams an uploaded chunk to S3 and records the part's ETag.
func (s *CloudDriveService) UploadPart(ctx context.Context, userID uint, sessionID string, partNumber int32, reader io.Reader, size int64) (string, *models.UploadSession, error) {
	var session models.UploadSession
	err := facades.Orm().Query().
		Where("session_id", sessionID).
		Where("user_id", userID).
		First(&session)
	if err != nil || session.ID == 0 {
		return "", nil, errors.New("upload session not found")
	}

	if session.ExpiresAt != nil && session.ExpiresAt.IsPast() {
		return "", nil, errors.New("upload session has expired")
	}

	driver, _, err := s.GetDriver(ctx, userID, session.CloudAccountID)
	if err != nil {
		return "", nil, err
	}

	etag, err := driver.UploadPart(ctx, session.StoragePath, session.UploadID, partNumber, reader, size)
	if err != nil {
		return "", nil, fmt.Errorf("failed to upload part %d: %w", partNumber, err)
	}

	if err := session.AddUploadedPart(contracts.CompletedPart{
		PartNumber: partNumber,
		ETag:       etag,
	}); err != nil {
		return "", nil, fmt.Errorf("failed to record uploaded part: %w", err)
	}

	if err := facades.Orm().Query().Save(&session); err != nil {
		return "", nil, fmt.Errorf("failed to update upload session: %w", err)
	}

	return etag, &session, nil
}

// CompleteMultipartUpload finalizes all parts on S3 and records the DriveItem.
func (s *CloudDriveService) CompleteMultipartUpload(ctx context.Context, userID uint, sessionID string) (*models.DriveItem, error) {
	var session models.UploadSession
	err := facades.Orm().Query().
		Where("session_id", sessionID).
		Where("user_id", userID).
		First(&session)
	if err != nil || session.ID == 0 {
		return nil, errors.New("upload session not found")
	}

	driver, account, err := s.GetDriver(ctx, userID, session.CloudAccountID)
	if err != nil {
		return nil, err
	}

	parts, err := session.GetUploadedParts()
	if err != nil || len(parts) == 0 {
		return nil, errors.New("no uploaded parts recorded for this session")
	}

	if err := driver.CompleteMultipart(ctx, session.StoragePath, session.UploadID, parts); err != nil {
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	meta, err := driver.GetMetadata(ctx, session.StoragePath)
	fileExt := strings.TrimPrefix(filepath.Ext(session.FileName), ".")
	etag := ""
	fileSize := session.TotalSize

	if err == nil && meta != nil {
		etag = meta.ETag
		if meta.Size > 0 {
			fileSize = meta.Size
		}
	}

	item := &models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         userID,
		CloudAccountID: session.CloudAccountID,
		ParentID:       session.ParentID,
		Name:           session.FileName,
		Type:           models.ItemTypeFile,
		MimeType:       session.MimeType,
		Size:           fileSize,
		Extension:      fileExt,
		StoragePath:    session.StoragePath,
		ETag:           etag,
		Status:         models.ItemStatusReady,
	}

	if err := facades.Orm().Query().Create(item); err != nil {
		return nil, fmt.Errorf("failed to create drive item: %w", err)
	}

	// Update used storage
	if account != nil && item.Size > 0 {
		account.UsedStorage += item.Size
		_ = facades.Orm().Query().Save(account)
	}

	// Cleanup session record
	_, _ = facades.Orm().Query().Where("id", session.ID).Delete(&models.UploadSession{})

	return item, nil
}

// AbortMultipartUpload aborts an in-progress multipart upload on S3 and removes session.
func (s *CloudDriveService) AbortMultipartUpload(ctx context.Context, userID uint, sessionID string) error {
	var session models.UploadSession
	err := facades.Orm().Query().
		Where("session_id", sessionID).
		Where("user_id", userID).
		First(&session)
	if err != nil || session.ID == 0 {
		return errors.New("upload session not found")
	}

	driver, _, err := s.GetDriver(ctx, userID, session.CloudAccountID)
	if err == nil {
		_ = driver.AbortMultipart(ctx, session.StoragePath, session.UploadID)
	}

	_, _ = facades.Orm().Query().Where("id", session.ID).Delete(&models.UploadSession{})
	return nil
}

// GetPresignedDownloadURL generates a temporary secure download URL for an item.
func (s *CloudDriveService) GetPresignedDownloadURL(ctx context.Context, userID uint, itemUUID string, expire time.Duration) (string, *models.DriveItem, error) {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return "", nil, err
	}

	if item.Type == models.ItemTypeFolder {
		return "", nil, errors.New("cannot generate download url for a folder")
	}

	driver, _, err := s.GetDriver(ctx, userID, item.CloudAccountID)
	if err != nil {
		return "", nil, err
	}

	url, err := driver.GetPresignedDownloadURL(ctx, item.StoragePath, item.Name, expire)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate presigned download url: %w", err)
	}

	return url, item, nil
}

// GetItemStream returns an io.ReadCloser to stream object content from S3.
func (s *CloudDriveService) GetItemStream(ctx context.Context, userID uint, itemUUID string) (io.ReadCloser, *models.DriveItem, error) {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return nil, nil, err
	}

	if item.Type == models.ItemTypeFolder {
		return nil, nil, errors.New("cannot stream a folder")
	}

	driver, _, err := s.GetDriver(ctx, userID, item.CloudAccountID)
	if err != nil {
		return nil, nil, err
	}

	body, err := driver.Get(ctx, item.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get storage object: %w", err)
	}

	return body, item, nil
}

// ScanBucket recursively scans an S3 bucket and synchronizes its objects into virtual DriveItem folders and files.
func (s *CloudDriveService) ScanBucket(ctx context.Context, userID uint, cloudAccountID uint, parentID *uint) (int, error) {
	driver, account, err := s.GetDriver(ctx, userID, cloudAccountID)
	if err != nil {
		return 0, err
	}

	syncedCount := 0
	var continuationToken string

	for {
		result, err := driver.ListObjects(ctx, "", continuationToken, 1000)
		if err != nil {
			return syncedCount, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range result.Objects {
			key := strings.TrimPrefix(obj.Key, "/")
			if key == "" {
				continue
			}

			// Check if this is an S3 directory marker (ends with /)
			isDirMarker := strings.HasSuffix(key, "/")
			trimmedKey := strings.Trim(key, "/")
			segments := strings.Split(trimmedKey, "/")
			if len(segments) == 0 {
				continue
			}

			currentParentID := parentID

			if isDirMarker {
				// Ensure all segments are created as folders
				for _, folderName := range segments {
					if folderName == "" {
						continue
					}
					var folder models.DriveItem
					q := facades.Orm().Query().
						Where("user_id", userID).
						Where("cloud_account_id", cloudAccountID).
						Where("name", folderName).
						Where("type", models.ItemTypeFolder)
					if currentParentID == nil {
						q = q.Where("parent_id IS NULL")
					} else {
						q = q.Where("parent_id = ?", *currentParentID)
					}
					_ = q.First(&folder)

					if folder.ID == 0 {
						folder = models.DriveItem{
							UUID:           uuid.New().String(),
							UserID:         userID,
							CloudAccountID: cloudAccountID,
							ParentID:       currentParentID,
							Name:           folderName,
							Type:           models.ItemTypeFolder,
							Status:         models.ItemStatusReady,
						}
						_ = facades.Orm().Query().Create(&folder)
					}
					fID := folder.ID
					currentParentID = &fID
				}
			} else {
				// Ensure parent directory folders exist
				for i := 0; i < len(segments)-1; i++ {
					folderName := segments[i]
					if folderName == "" {
						continue
					}
					var folder models.DriveItem
					q := facades.Orm().Query().
						Where("user_id", userID).
						Where("cloud_account_id", cloudAccountID).
						Where("name", folderName).
						Where("type", models.ItemTypeFolder)
					if currentParentID == nil {
						q = q.Where("parent_id IS NULL")
					} else {
						q = q.Where("parent_id = ?", *currentParentID)
					}
					_ = q.First(&folder)

					if folder.ID == 0 {
						folder = models.DriveItem{
							UUID:           uuid.New().String(),
							UserID:         userID,
							CloudAccountID: cloudAccountID,
							ParentID:       currentParentID,
							Name:           folderName,
							Type:           models.ItemTypeFolder,
							Status:         models.ItemStatusReady,
						}
						_ = facades.Orm().Query().Create(&folder)
					}
					fID := folder.ID
					currentParentID = &fID
				}

				// Upsert the file
				fileName := segments[len(segments)-1]
				fileExt := strings.TrimPrefix(filepath.Ext(fileName), ".")

				var existing models.DriveItem
				_ = facades.Orm().Query().
					Where("user_id", userID).
					Where("cloud_account_id", cloudAccountID).
					Where("storage_path", obj.Key).
					First(&existing)

				if existing.ID > 0 {
					existing.Size = obj.Size
					existing.ETag = obj.ETag
					existing.Status = models.ItemStatusReady
					existing.ParentID = currentParentID
					_ = facades.Orm().Query().Save(&existing)
				} else {
					var nameMatch models.DriveItem
					q := facades.Orm().Query().
						Where("user_id", userID).
						Where("cloud_account_id", cloudAccountID).
						Where("name", fileName).
						Where("type", models.ItemTypeFile)
					if currentParentID == nil {
						q = q.Where("parent_id IS NULL")
					} else {
						q = q.Where("parent_id = ?", *currentParentID)
					}
					_ = q.First(&nameMatch)

					if nameMatch.ID > 0 {
						nameMatch.StoragePath = obj.Key
						nameMatch.Size = obj.Size
						nameMatch.ETag = obj.ETag
						nameMatch.Status = models.ItemStatusReady
						_ = facades.Orm().Query().Save(&nameMatch)
					} else {
						newItem := models.DriveItem{
							UUID:           uuid.New().String(),
							UserID:         userID,
							CloudAccountID: cloudAccountID,
							ParentID:       currentParentID,
							Name:           fileName,
							Type:           models.ItemTypeFile,
							MimeType:       obj.MimeType,
							Size:           obj.Size,
							Extension:      fileExt,
							StoragePath:    obj.Key,
							ETag:           obj.ETag,
							Status:         models.ItemStatusReady,
						}
						_ = facades.Orm().Query().Create(&newItem)
					}
				}
				syncedCount++
			}
		}

		if !result.IsTruncated || result.NextContinuation == "" {
			break
		}
		continuationToken = result.NextContinuation
	}

	// Recalculate and update used storage for account
	if account != nil {
		var totalUsed int64
		_ = facades.Orm().Query().Model(&models.DriveItem{}).
			Where("user_id", userID).
			Where("cloud_account_id", cloudAccountID).
			Where("type", models.ItemTypeFile).
			Select("COALESCE(SUM(size), 0)").
			Scan(&totalUsed)
		account.UsedStorage = totalUsed
		_ = facades.Orm().Query().Save(account)
	}

	return syncedCount, nil
}
