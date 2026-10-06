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

// ResolveCloudAccountID maps a public account uuid to the numeric id the rest of
// the service uses. The API speaks uuid; internal queries keep using id.
func (s *CloudDriveService) ResolveCloudAccountID(ctx context.Context, userID uint, accountUUID string) (uint, error) {
	if accountUUID == "" {
		return 0, errors.New("cloud account uuid is required")
	}

	var account models.CloudAccount
	err := facades.Orm().Query().
		Where("uuid", accountUUID).
		Where("user_id", userID).
		First(&account)
	if err != nil || account.ID == 0 {
		return 0, errors.New("cloud account not found or access denied")
	}

	return account.ID, nil
}

// ResolveDriveItemID maps a public item uuid to its numeric id, scoped to the
// user and (when non-empty) the owning account.
func (s *CloudDriveService) ResolveDriveItemID(ctx context.Context, userID uint, itemUUID string) (uint, error) {
	if itemUUID == "" {
		return 0, errors.New("item uuid is required")
	}

	var item models.DriveItem
	err := facades.Orm().Query().
		Where("uuid", itemUUID).
		Where("user_id", userID).
		First(&item)
	if err != nil || item.ID == 0 {
		return 0, errors.New("item not found")
	}

	return item.ID, nil
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

// ListItems queries one page of items for a given cloud account and parent
// directory.
//
// Results are ordered folders-first, then by sortField/sortOrder, then by id.
// The id tier is not decoration: without it the ordering is not total, and rows
// that share a sort value - routine for updated_at, which has one-second
// granularity - would shuffle between pages, so a paging client would see some
// items twice and never see others.
//
// An empty cursor means the first page. A non-empty limit is required; callers
// pass limit+1 and trim, so has_more can be decided without a COUNT.
func (s *CloudDriveService) ListItems(ctx context.Context, userID uint, cloudAccountID uint, parentID *uint, search string, itemType string, sortField string, sortOrder string, cursor DriveCursor, limit int) ([]models.DriveItem, error) {
	orderCol := NormalizeSortField(sortField)
	orderDir := NormalizeSortOrder(sortOrder)

	query := facades.Orm().Query().
		Where("user_id", userID).
		Where("cloud_account_id", cloudAccountID)

	// A search spans the whole account rather than the folder in view, so the
	// parent_id constraint is dropped when a term is present. Browsing (no
	// term) stays scoped to the current folder.
	trimmedSearch := strings.TrimSpace(search)
	if trimmedSearch != "" {
		query = query.Where("name LIKE ?", "%"+trimmedSearch+"%")
	} else if parentID == nil || *parentID == 0 {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id", *parentID)
	}

	if itemType == models.ItemTypeFolder || itemType == models.ItemTypeFile {
		query = query.Where("type", itemType)
	}

	// A cursor only means something inside the ordering that produced it. If
	// the caller changed `sort` or `order` since, the position is meaningless
	// and is ignored, so the request degrades to a first page instead of
	// comparing, say, a name against the size column.
	if cursor.ID > 0 && cursor.Matches(orderCol, orderDir) {
		cond, args := BuildCursorWhere(cursor, orderCol, orderDir)
		query = query.Where(cond, args...)
	}

	if limit <= 0 {
		limit = 50
	}

	// ORDER BY type DESC replaces the old
	// `CASE WHEN type = 'folder' THEN 0 ELSE 1 END`: 'file' sorts before
	// 'folder' alphabetically, so descending puts folders first while still
	// being a plain column reference the index can serve.
	var items []models.DriveItem
	if err := query.
		Order("type DESC").
		Order(orderCol + " " + orderDir).
		Order("id ASC").
		Limit(limit).
		Get(&items); err != nil {
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

// GetBreadcrumb returns the ancestor chain for a folder, ordered root first and
// ending with the folder itself. A client renders it as the breadcrumb and uses
// the first entry to know it is at the top.
//
// The walk is bounded by a visited set: a corrupted `parent_id` cycle would
// otherwise loop forever, and a depth cap keeps a pathologically deep tree from
// issuing an unbounded number of queries.
func (s *CloudDriveService) GetBreadcrumb(ctx context.Context, userID uint, itemUUID string) ([]models.DriveItem, error) {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return nil, err
	}

	const maxDepth = 100
	chain := []models.DriveItem{*item}
	visited := map[uint]bool{item.ID: true}

	current := item
	for current.ParentID != nil && *current.ParentID > 0 {
		if len(chain) >= maxDepth {
			break
		}

		parentID := *current.ParentID
		if visited[parentID] {
			// A cycle: stop rather than loop. The chain stays usable.
			break
		}

		var parent models.DriveItem
		if err := facades.Orm().Query().
			Where("id", parentID).
			Where("user_id", userID).
			First(&parent); err != nil || parent.ID == 0 {
			// A dangling parent (deleted out from under us): stop at the last
			// resolvable entry rather than fail the whole breadcrumb.
			break
		}

		visited[parent.ID] = true
		chain = append(chain, parent)
		current = &parent
	}

	// The walk produced child -> ... -> root; the caller wants root first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain, nil
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
				Where("cloud_account_id", item.CloudAccountID).
				First(&parent)
			if err != nil || parent.ID == 0 {
				return nil, errors.New("destination folder not found")
			}
			if !parent.IsFolder() {
				return nil, errors.New("destination is not a folder")
			}
			// Prevent cycles: the destination must not be the item itself or one
			// of its own descendants. Walk up from the destination; if we reach
			// the item, the move would create a loop.
			if s.isSelfOrDescendant(userID, item.CloudAccountID, *newParentID, item.ID) {
				return nil, errors.New("cannot move a folder into itself or its own subfolder")
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

// isSelfOrDescendant reports whether startID equals targetID or has targetID as
// an ancestor. It walks the parent chain upward, guarded by a visited set
// against pre-existing cycles and a depth cap against runaway trees.
func (s *CloudDriveService) isSelfOrDescendant(userID uint, cloudAccountID uint, startID uint, targetID uint) bool {
	const maxDepth = 10000
	visited := map[uint]bool{}
	current := startID
	for depth := 0; current != 0 && depth < maxDepth; depth++ {
		if current == targetID {
			return true
		}
		if visited[current] {
			break
		}
		visited[current] = true

		var node models.DriveItem
		if err := facades.Orm().Query().
			Where("id", current).
			Where("user_id", userID).
			Where("cloud_account_id", cloudAccountID).
			First(&node); err != nil || node.ID == 0 {
			break
		}
		if node.ParentID == nil {
			break
		}
		current = *node.ParentID
	}
	return false
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

// DeleteItem deletes an item (soft-delete or permanent delete). For folders with
// permanent=true, all descendant files and folders are recursively deleted and
// used_storage is decremented.
func (s *CloudDriveService) DeleteItem(ctx context.Context, userID uint, itemUUID string, permanent bool) error {
	item, err := s.GetItemByUUID(ctx, userID, itemUUID)
	if err != nil {
		return err
	}

	if !permanent {
		_, err = facades.Orm().Query().Delete(item)
		return err
	}

	// Permanent delete path
	driver, account, driverErr := s.GetDriver(ctx, userID, item.CloudAccountID)
	if driverErr != nil {
		// Log internally only: the S3 delete is skipped, but the row deletion below
		// still proceeds so the item is not orphaned in the DB. The error is never
		// returned to the client.
		facades.Log().Warningf("[Drive] Failed to initialize storage driver for item %s (user %d) during permanent delete: %v", itemUUID, userID, driverErr)
	}

	if item.IsFile() {
		if driverErr == nil && driver != nil && item.StoragePath != "" {
			_ = driver.Delete(ctx, item.StoragePath)
		}
		if _, err := facades.Orm().Query().ForceDelete(item); err != nil {
			return err
		}
		if account != nil && item.Size > 0 {
			newUsed := account.UsedStorage - item.Size
			if newUsed < 0 {
				newUsed = 0
			}
			account.UsedStorage = newUsed
			_ = facades.Orm().Query().Save(account)
		}
		return nil
	}

	// Folder permanent delete: collect all descendants
	allItems, keys, totalSize, collectErr := s.CollectDescendantItems(ctx, userID, item.CloudAccountID, item.ID)
	if collectErr != nil {
		return collectErr
	}

	// Batch delete from S3 driver
	if driverErr == nil && driver != nil && len(keys) > 0 {
		if batchErr := driver.DeleteObjects(ctx, keys); batchErr != nil {
			facades.Log().Warningf("[Drive] Failed to batch delete %d objects for folder %s: %v", len(keys), itemUUID, batchErr)
		}
	}

	// Force delete rows in batch
	ids := make([]uint, 0, len(allItems))
	for _, it := range allItems {
		ids = append(ids, it.ID)
	}

	if len(ids) > 0 {
		if _, err := facades.Orm().Query().Where("id IN (?)", ids).ForceDelete(&models.DriveItem{}); err != nil {
			return err
		}
	}

	// Decrement used_storage floored at 0
	if account != nil && totalSize > 0 {
		newUsed := account.UsedStorage - totalSize
		if newUsed < 0 {
			newUsed = 0
		}
		account.UsedStorage = newUsed
		_ = facades.Orm().Query().Save(account)
	}

	return nil
}

// CollectDescendantItems walks the folder subtree via BFS starting from rootFolderID,
// guarded by a visited set against cycles. It returns all collected DriveItems (including
// root), all non-empty file storage keys, and the total size of files.
func (s *CloudDriveService) CollectDescendantItems(ctx context.Context, userID uint, cloudAccountID uint, rootFolderID uint) ([]models.DriveItem, []string, int64, error) {
	var root models.DriveItem
	err := facades.Orm().Query().
		Where("id", rootFolderID).
		Where("user_id", userID).
		Where("cloud_account_id", cloudAccountID).
		First(&root)
	if err != nil || root.ID == 0 {
		return nil, nil, 0, errors.New("root item not found")
	}

	queue := []uint{root.ID}
	visited := map[uint]bool{root.ID: true}
	allItems := []models.DriveItem{root}
	var keys []string
	var totalSize int64

	const maxItems = 10000

	for len(queue) > 0 {
		if len(allItems) >= maxItems {
			break
		}
		currentID := queue[0]
		queue = queue[1:]

		var children []models.DriveItem
		err := facades.Orm().Query().
			Where("user_id", userID).
			Where("cloud_account_id", cloudAccountID).
			Where("parent_id", currentID).
			Get(&children)
		if err != nil {
			return nil, nil, 0, err
		}

		for _, child := range children {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true
			allItems = append(allItems, child)

			if child.IsFolder() {
				queue = append(queue, child.ID)
			} else if child.IsFile() {
				if child.StoragePath != "" {
					keys = append(keys, child.StoragePath)
				}
				if child.Size > 0 {
					totalSize += child.Size
				}
			}
		}
	}

	return allItems, keys, totalSize, nil
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

			// An object the app itself uploaded is stored flat under the reserved
			// internal prefix (users/<id>/items/<uuid>-<name>) and already has a
			// DriveItem row carrying the user's real ParentID. That key path is a
			// storage detail, not a user-visible folder tree, so never derive
			// folders from it and never re-parent the item: the DB row is
			// authoritative. Refresh only the metadata that S3 owns.
			var tracked models.DriveItem
			_ = facades.Orm().Query().
				Where("user_id", userID).
				Where("cloud_account_id", cloudAccountID).
				Where("storage_path", obj.Key).
				First(&tracked)
			if tracked.ID > 0 {
				tracked.Size = obj.Size
				tracked.ETag = obj.ETag
				tracked.Status = models.ItemStatusReady
				_ = facades.Orm().Query().Save(&tracked)
				syncedCount++
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

				// Not tracked by storage_path (checked above), so match an
				// existing row by name within the derived folder, else create.
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
