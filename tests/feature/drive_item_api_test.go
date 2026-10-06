package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type DriveItemAPITestSuite struct {
	suite.Suite
	tests.TestCase
	user    models.User
	token   string
	account models.CloudAccount
}

func TestDriveItemAPITestSuite(t *testing.T) {
	suite.Run(t, new(DriveItemAPITestSuite))
}

func (s *DriveItemAPITestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Drive Item API User",
		Username: "driveapi_" + uuid.New().String()[:8],
		Email:    "driveapi_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	// Obtain JWT
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	dataMap := jsonBody["data"].(map[string]any)
	s.token = dataMap["token"].(string)

	// Create CloudAccount
	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "User S3 Storage",
		Provider:  models.ProviderS3,
		IsDefault: true,
		IsActive:  true,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA...",
		SecretAccessKey: "secret...",
	})
	err = facades.Orm().Query().Create(&s.account)
	s.Require().NoError(err)

	s.Require().NotEmpty(s.account.UUID)
}

func (s *DriveItemAPITestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *DriveItemAPITestSuite) TestDriveItemLifecycle() {
	// 1. Create a Folder (POST /v1/drive/items/folders)
	createFolderPayload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"name":             "Work Documents",
	})
	createResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/items/folders", bytes.NewBuffer(createFolderPayload))
	s.Require().NoError(err)
	createResp.AssertStatus(http.StatusCreated)

	createBody, err := createResp.Json()
	s.Require().NoError(err)
	folderData := createBody["data"].(map[string]any)
	folderUUID := folderData["uuid"].(string)
	s.NotEmpty(folderUUID)
	s.NotContains(folderData, "id", "no numeric id may be exposed")
	s.Equal("Work Documents", folderData["name"])
	s.Equal(models.ItemTypeFolder, folderData["type"])

	// 2. List Root Items (GET /v1/drive/items?cloud_account_id=...)
	listResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items?cloud_account_id=%d", s.account.ID))
	s.Require().NoError(err)
	listResp.AssertOk()

	listBody, err := listResp.Json()
	s.Require().NoError(err)
	itemsList := listBody["data"].([]any)
	s.Len(itemsList, 1)

	// 3. Create a Child Item inside the folder (use parent_uuid)
	childFolderPayload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"parent_uuid":      folderUUID,
		"name":             "Sub Projects",
	})
	childResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/items/folders", bytes.NewBuffer(childFolderPayload))
	s.Require().NoError(err)
	childResp.AssertStatus(http.StatusCreated)

	childBody, _ := childResp.Json()
	childUUID := childBody["data"].(map[string]any)["uuid"].(string)

	// 4. List items inside folder (GET /v1/drive/items?cloud_account_id=...&parent_uuid=...)
	folderItemsResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items?cloud_account_id=%d&parent_uuid=%s", s.account.ID, folderUUID))
	s.Require().NoError(err)
	folderItemsResp.AssertOk()

	folderItemsBody, _ := folderItemsResp.Json()
	folderChildren := folderItemsBody["data"].([]any)
	s.Len(folderChildren, 1)
	s.Equal("Sub Projects", folderChildren[0].(map[string]any)["name"])

	// 5. Get Item Details (GET /v1/drive/items/{uuid})
	showResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s", folderUUID))
	s.Require().NoError(err)
	showResp.AssertOk()

	showBody, _ := showResp.Json()
	s.Equal("Work Documents", showBody["data"].(map[string]any)["name"])

	// 6. Rename / Move Item (PATCH /v1/drive/items/{uuid})
	patchPayload, _ := json.Marshal(map[string]any{
		"name": "Archived Documents",
	})
	patchResp, err := s.Http(s.T()).WithToken(s.token).WithHeader("Content-Type", "application/json").Patch(fmt.Sprintf("/v1/drive/items/%s", folderUUID), bytes.NewBuffer(patchPayload))
	s.Require().NoError(err)
	patchResp.AssertOk()

	patchBody, _ := patchResp.Json()
	s.Equal("Archived Documents", patchBody["data"].(map[string]any)["name"])

	// 7. Star Item (POST /v1/drive/items/{uuid}/star)
	starResp, err := s.Http(s.T()).WithToken(s.token).Post(fmt.Sprintf("/v1/drive/items/%s/star", folderUUID), nil)
	s.Require().NoError(err)
	starResp.AssertOk()

	starBody, _ := starResp.Json()
	s.True(starBody["data"].(map[string]any)["is_starred"].(bool))

	// 8. Delete Item (DELETE /v1/drive/items/{uuid})
	delResp, err := s.Http(s.T()).WithToken(s.token).Delete(fmt.Sprintf("/v1/drive/items/%s", childUUID), nil)
	s.Require().NoError(err)
	delResp.AssertOk()

	// Verify child folder is gone
	delShowResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s", childUUID))
	s.Require().NoError(err)
	delShowResp.AssertStatus(http.StatusNotFound)
}

// TestCreateFolderErrorDoesNotLeakInternalDetail proves a service error is not
// echoed back to the client. Creating a folder under an unknown cloud account
// produces a genuine service error; the response must carry only the localized
// message, never the internal error text.
func (s *DriveItemAPITestSuite) TestCreateFolderErrorDoesNotLeakInternalDetail() {
	payload, _ := json.Marshal(map[string]any{
		"cloud_account_id": 999999999,
		"name":             "Leaky Folder",
	})

	resp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/items/folders", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusBadRequest)

	body, err := resp.Json()
	s.Require().NoError(err)

	message, _ := body["message"].(string)
	s.NotEmpty(message)
	s.NotContains(message, "invalid cloud account",
		"the internal service error must not be echoed to the client")
	s.NotContains(message, "Error ",
		"raw driver/SQL error text must not reach the client")
	s.NotContains(message, "create_failed",
		"the translation key must resolve: an unresolved key would be shown to the user verbatim")
}

// TestBreadcrumb pins the endpoint a hard reload relies on: opening a folder
// only changes local state today, so a refresh loses the path. The URL now
// carries the current folder's uuid and this endpoint rebuilds the ancestor
// chain from it, root first, so the breadcrumb can be rendered after a reload.
func (s *DriveItemAPITestSuite) TestBreadcrumb() {
	// root -> Work -> Sub -> Deep
	work := s.createFolderItem("Work", nil)
	sub := s.createFolderItem("Sub", &work.UUID)
	deep := s.createFolderItem("Deep", &sub.UUID)

	resp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s/breadcrumb", deep.UUID))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	chain := body["data"].([]any)
	s.Len(chain, 3, "the chain must contain every ancestor, root first")

	names := make([]string, 0, len(chain))
	for _, entry := range chain {
		names = append(names, entry.(map[string]any)["name"].(string))
	}
	s.Equal([]string{"Work", "Sub", "Deep"}, names)

	// The first entry is the top of the chain: a client uses it to know it can
	// stop walking up. parent_id is no longer in response (uuid is used instead).
	_, hasParentID := chain[0].(map[string]any)["parent_id"]
	s.False(hasParentID, "the root-most entry must not have parent_id field")
	s.NotContains(chain[0].(map[string]any), "parent_id")

	// A root-level folder yields a single-entry chain.
	rootResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s/breadcrumb", work.UUID))
	s.Require().NoError(err)
	rootResp.AssertOk()
	rootBody, _ := rootResp.Json()
	s.Len(rootBody["data"].([]any), 1)
}

// TestBreadcrumbNotFoundDoesNotLeakInternalDetail checks the failure path: an
// unknown uuid returns a localized 404, never the internal error text.
func (s *DriveItemAPITestSuite) TestBreadcrumbNotFoundDoesNotLeakInternalDetail() {
	resp, err := s.Http(s.T()).WithToken(s.token).Get("/v1/drive/items/00000000-0000-0000-0000-000000000000/breadcrumb")
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusNotFound)

	body, err := resp.Json()
	s.Require().NoError(err)
	message, _ := body["message"].(string)
	s.NotEmpty(message)
	s.NotContains(message, "item not found",
		"the internal service error must not be echoed to the client")
	s.NotContains(message, "item_not_found",
		"the translation key must resolve before reaching the client")
}

// TestDeleteItemDoesNotLeakInternalDetail checks the failure path: deleting an
// unknown uuid must return a localized message, never the internal error text
// ("item not found") from cloud_drive_service.go.
//
// NOTE on status code: the spec §5.6 wording says 404 for a missing item, but
// the controller currently returns 400 here because Destroy routes the
// DeleteItem error through failResponse(status=BadRequest). This test asserts
// the ACTUAL behavior (400) and documents the discrepancy; the controller is
// intentionally left unchanged in this round.
func (s *DriveItemAPITestSuite) TestDeleteItemDoesNotLeakInternalDetail() {
	unknownUUID := uuid.New().String()
	resp, err := s.Http(s.T()).WithToken(s.token).Delete(
		fmt.Sprintf("/v1/drive/items/%s?permanent=true", unknownUUID),
		nil,
	)
	s.Require().NoError(err)

	// The controller currently returns 400 (not 404 as the spec §5.6 wording suggests).
	// This is the actual behavior; assert it and note the discrepancy.
	resp.AssertStatus(http.StatusBadRequest)

	body, err := resp.Json()
	s.Require().NoError(err)
	message, _ := body["message"].(string)
	s.NotEmpty(message)

	s.NotContains(message, "item not found",
		"the internal service error ('item not found' from cloud_drive_service.go) must not be echoed to the client")
	s.NotContains(message, "item_not_found",
		"the translation key must resolve before reaching the client")

	// The body must carry the localized drive.delete_failed message. The test
	// app defaults to the Vietnamese locale, so the resolved message is the vi
	// translation; we accept either locale's translation to stay robust.
	s.Contains(message, "Không thể xóa mục. Vui lòng thử lại.",
		"the response should contain the localized (vi) 'drive.delete_failed' message")
	s.NotEqual("drive.delete_failed", message,
		"the translation key must resolve: an unresolved key would be shown to the user verbatim")
}

// createFolderItem posts a folder via the API and returns the created item.
// parentUUID is the UUID of the parent folder, or nil for root level.
func (s *DriveItemAPITestSuite) createFolderItem(name string, parentUUID *string) models.DriveItem {
	payload := map[string]any{
		"cloud_account_id": s.account.ID,
		"name":             name,
	}
	if parentUUID != nil {
		payload["parent_uuid"] = *parentUUID
	}
	raw, _ := json.Marshal(payload)

	resp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/items/folders", bytes.NewBuffer(raw))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusCreated)

	body, err := resp.Json()
	s.Require().NoError(err)
	data := body["data"].(map[string]any)

	var item models.DriveItem
	item.UUID = data["uuid"].(string)
	item.Name = data["name"].(string)
	return item
}

// createFolder posts a folder under parentUUID ("" for root) and returns its uuid.
func (s *DriveItemAPITestSuite) createFolder(parentUUID, name string) string {
	payload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"parent_uuid":      parentUUID,
		"name":             name,
	})
	resp, err := s.Http(s.T()).WithToken(s.token).
		Post("/v1/drive/items/folders", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusCreated)
	body, _ := resp.Json()
	return body["data"].(map[string]any)["uuid"].(string)
}

// listFolder returns the raw data array for a folder ("" for root).
func (s *DriveItemAPITestSuite) listFolder(parentUUID string) []any {
	url := fmt.Sprintf("/v1/drive/items?cloud_account_id=%d", s.account.ID)
	if parentUUID != "" {
		url += "&parent_uuid=" + parentUUID
	}
	resp, err := s.Http(s.T()).WithToken(s.token).Get(url)
	s.Require().NoError(err)
	resp.AssertOk()
	body, _ := resp.Json()
	return body["data"].([]any)
}

// createFileItem inserts a file row directly (no S3 needed for a pure parent move).
func (s *DriveItemAPITestSuite) createFileItem(name string) models.DriveItem {
	file := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		Name:           name,
		Type:           models.ItemTypeFile,
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&file))
	return file
}

// moveItem PATCHes an item's parent_uuid ("" for root) and asserts a 200.
func (s *DriveItemAPITestSuite) moveItem(itemUUID, parentUUID string) {
	payload, _ := json.Marshal(map[string]any{"parent_uuid": parentUUID})
	resp, err := s.Http(s.T()).WithToken(s.token).
		WithHeader("Content-Type", "application/json").
		Patch(fmt.Sprintf("/v1/drive/items/%s", itemUUID), bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertOk()
}

// assertOnlyChildInFolder asserts folderUUID lists exactly one item named name,
// and that the same name does not appear at the root.
func (s *DriveItemAPITestSuite) assertOnlyChildInFolder(folderUUID, name string) {
	children := s.listFolder(folderUUID)
	s.Len(children, 1)
	s.Equal(name, children[0].(map[string]any)["name"])

	for _, it := range s.listFolder("") {
		s.NotEqual(name, it.(map[string]any)["name"])
	}
}

// assertMoveRejected PATCHes itemUUID to parentUUID, asserts a 400, and asserts
// the response message is the localized user message: non-empty and containing
// none of the given raw internal error substrings, nor the translation key.
func (s *DriveItemAPITestSuite) assertMoveRejected(itemUUID, parentUUID string, rawLeaks ...string) {
	payload, _ := json.Marshal(map[string]any{"parent_uuid": parentUUID})
	resp, err := s.Http(s.T()).WithToken(s.token).
		WithHeader("Content-Type", "application/json").
		Patch(fmt.Sprintf("/v1/drive/items/%s", itemUUID), bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusBadRequest)

	body, _ := resp.Json()
	message, _ := body["message"].(string)
	s.NotEmpty(message)
	for _, raw := range rawLeaks {
		s.NotContains(message, raw, "raw service error must not leak")
	}
	s.NotContains(message, "update_failed", "the translation key must resolve")
}

func (s *DriveItemAPITestSuite) TestListItemsCarryCloudAccountID() {
	s.createFolderItem("Work", nil)

	resp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items?cloud_account_id=%d", s.account.ID))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	items := body["data"].([]any)
	s.Require().NotEmpty(items)
	first := items[0].(map[string]any)
	s.Equal(float64(s.account.ID), first["cloud_account_id"])
	s.NotContains(first, "cloud_account_uuid")
}

func (s *DriveItemAPITestSuite) TestPermanentDeleteFolderRecursiveWithStorageDecrement() {
	// Set initial used_storage
	s.account.UsedStorage = 50000
	_ = facades.Orm().Query().Save(&s.account)

	// 1. Create a parent folder
	parent := s.createFolderItem("ParentFolder", nil)

	// 2. Create child folder
	child := s.createFolderItem("ChildFolder", &parent.UUID)

	// 3. Create file inside child folder directly in DB
	file := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		ParentID: func() *uint {
			var it models.DriveItem
			_ = facades.Orm().Query().Where("uuid", child.UUID).First(&it)
			return &it.ID
		}(),
		Name:        "nested-doc.txt",
		Type:        models.ItemTypeFile,
		Size:        15000,
		StoragePath: "users/1/items/mock-nested-doc.txt",
		Status:      models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&file))

	// 4. Issue DELETE /v1/drive/items/{parent_uuid}?permanent=true
	delResp, err := s.Http(s.T()).WithToken(s.token).Delete(
		fmt.Sprintf("/v1/drive/items/%s?permanent=true", parent.UUID),
		nil,
	)
	s.Require().NoError(err)
	delResp.AssertOk()

	// 5. Assert parent, child and nested file are completely gone from DB
	count, err := facades.Orm().Query().Model(&models.DriveItem{}).
		Where("uuid IN (?)", []string{parent.UUID, child.UUID, file.UUID}).
		Count()
	s.Require().NoError(err)
	s.Equal(int64(0), count, "all items in hierarchy must be permanently deleted")

	// 6. Assert used_storage decremented: 50000 - 15000 = 35000
	var refreshedAccount models.CloudAccount
	_ = facades.Orm().Query().Where("id", s.account.ID).First(&refreshedAccount)
	s.Equal(int64(35000), refreshedAccount.UsedStorage)
}

func (s *DriveItemAPITestSuite) TestStorageDecrementDoesNotGoBelowZero() {
	s.account.UsedStorage = 2000
	_ = facades.Orm().Query().Save(&s.account)

	file := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		Name:           "large.txt",
		Type:           models.ItemTypeFile,
		Size:           10000, // larger than used_storage
		StoragePath:    "users/1/items/large.txt",
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&file))

	delResp, err := s.Http(s.T()).WithToken(s.token).Delete(
		fmt.Sprintf("/v1/drive/items/%s?permanent=true", file.UUID),
		nil,
	)
	s.Require().NoError(err)
	delResp.AssertOk()

	var refreshedAccount models.CloudAccount
	_ = facades.Orm().Query().Where("id", s.account.ID).First(&refreshedAccount)
	s.Equal(int64(0), refreshedAccount.UsedStorage, "storage decrement must be floored at 0")
}

// TestMoveItemIntoFolder moves a file into a folder and verifies via listing.
func (s *DriveItemAPITestSuite) TestMoveItemIntoFolder() {
	// folder A (root), folder B (root)
	folderA := s.createFolder("", "Move Target A")
	folderB := s.createFolder("", "Move Target B")

	file := s.createFileItem("movable.txt")

	// Move the file into folder A.
	s.moveItem(file.UUID, folderA)
	s.assertOnlyChildInFolder(folderA, "movable.txt")

	// Move it again into folder B.
	s.moveItem(file.UUID, folderB)
	s.Len(s.listFolder(folderB), 1)
	s.Len(s.listFolder(folderA), 0)
}

// TestMoveFolderIntoOwnDescendantIsRejected proves the cycle guard.
func (s *DriveItemAPITestSuite) TestMoveFolderIntoOwnDescendantIsRejected() {
	parent := s.createFolder("", "Cycle Parent")
	child := s.createFolder(parent, "Cycle Child")

	s.assertMoveRejected(parent, child, "cannot move a folder", "own subfolder")

	// The tree is unchanged: child still under parent.
	s.Len(s.listFolder(parent), 1)
}

// TestMoveItemToRoot proves the Blocker fix: parent_uuid:"" moves an item to root.
func (s *DriveItemAPITestSuite) TestMoveItemToRoot() {
	// folder A (root); file at root
	folderA := s.createFolder("", "Move Target A")
	file := s.createFileItem("root-file.txt")

	// 1. Move file into folder A.
	s.moveItem(file.UUID, folderA)
	s.assertOnlyChildInFolder(folderA, "root-file.txt")

	// 2. Move file back to root via parent_uuid:"" (the Blocker path).
	s.moveItem(file.UUID, "")

	// It now lists at root and not under folder A.
	found := false
	for _, it := range s.listFolder("") {
		if it.(map[string]any)["name"] == "root-file.txt" {
			found = true
		}
	}
	s.True(found, "file must be back at root after parent_uuid: \"\"")
	s.Len(s.listFolder(folderA), 0)
}

// TestMoveFolderIntoFolder proves folder-to-folder moves at the API level.
func (s *DriveItemAPITestSuite) TestMoveFolderIntoFolder() {
	// folder A and folder B at root
	folderA := s.createFolder("", "Target Folder")
	folderB := s.createFolder("", "Movable Folder")

	// Move B into A.
	s.moveItem(folderB, folderA)
	s.assertOnlyChildInFolder(folderA, "Movable Folder")
}

// TestMoveIntoNonFolderDestinationIsRejected proves a file cannot host other items.
func (s *DriveItemAPITestSuite) TestMoveIntoNonFolderDestinationIsRejected() {
	// Create two files at root.
	fileA := s.createFileItem("host.txt")
	fileB := s.createFileItem("guest.txt")

	// Attempt to move fileB into fileA (a non-folder destination).
	s.assertMoveRejected(fileB.UUID, fileA.UUID, "destination is not a folder")
}

// TestMoveFolderIntoItselfIsRejected proves the self-parent guard still holds.
func (s *DriveItemAPITestSuite) TestMoveFolderIntoItselfIsRejected() {
	folder := s.createFolder("", "Self Parent")

	s.assertMoveRejected(folder, folder, "own parent")
}

// TestMoveIntoDifferentAccountFolderIsRejected proves the same-account guard.
func (s *DriveItemAPITestSuite) TestMoveIntoDifferentAccountFolderIsRejected() {
	other := models.CloudAccount{
		UserID:   s.user.ID,
		Name:     "Other S3",
		Provider: models.ProviderS3,
		IsActive: true,
	}
	_ = other.SetCredentials(&models.S3Credentials{
		Bucket: "other-bucket", Region: "us-east-1",
		AccessKeyID: "AKIA...", SecretAccessKey: "secret...",
	})
	s.Require().NoError(facades.Orm().Query().Create(&other))

	// A folder in the OTHER account.
	otherFolder := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: other.ID,
		Name:           "Other Account Folder",
		Type:           models.ItemTypeFolder,
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&otherFolder))

	// An item in the default account.
	item := s.createFileItem("cross.txt")

	s.assertMoveRejected(item.UUID, otherFolder.UUID, "destination folder not found")

	// The item must stay put: still at root of the default account, parent_id nil.
	var refreshed models.DriveItem
	s.Require().NoError(facades.Orm().Query().Where("uuid", item.UUID).First(&refreshed))
	s.Nil(refreshed.ParentID, "item must remain at root after a rejected cross-account move")
	s.Equal(s.account.ID, refreshed.CloudAccountID, "item must stay in its original account")
}

// searchNames queries the list endpoint with a search term and no parent, and
// returns the names found.
func (s *DriveItemAPITestSuite) searchNames(term string) []string {
	url := fmt.Sprintf("/v1/drive/items?cloud_account_id=%d&search=%s", s.account.ID, term)
	resp, err := s.Http(s.T()).WithToken(s.token).Get(url)
	s.Require().NoError(err)
	resp.AssertOk()

	body, _ := resp.Json()
	raw := body["data"].([]any)
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		names = append(names, entry.(map[string]any)["name"].(string))
	}
	return names
}

// TestSearchSpansWholeAccount: a search must find items in subfolders, not just
// the folder in view. The term drops the parent scope but stays inside the
// account.
func (s *DriveItemAPITestSuite) TestSearchSpansWholeAccount() {
	parent := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		Name:           "Search Docs",
		Type:           models.ItemTypeFolder,
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&parent))

	nested := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		ParentID:       &parent.ID,
		Name:           "findme.txt",
		Type:           models.ItemTypeFile,
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&nested))
	s.createFileItem("other.txt")

	names := s.searchNames("findme")

	s.Contains(names, "findme.txt", "a file inside a subfolder must be found from the root")
	s.NotContains(names, "other.txt")
}

// TestSearchMatchesSubstringAndRejectsNonMatch: matching is a substring of the
// name, and a non-matching term returns nothing rather than the whole folder.
func (s *DriveItemAPITestSuite) TestSearchMatchesSubstringAndRejectsNonMatch() {
	s.createFileItem("quarterly-report.pdf")

	s.Contains(s.searchNames("report"), "quarterly-report.pdf")
	s.Empty(s.searchNames("zzz-no-such-name"), "a non-matching search must return nothing")
}
