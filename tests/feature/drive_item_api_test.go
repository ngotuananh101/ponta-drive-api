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
	_ = folderUUID // use folderUUID for parent_uuid in subsequent calls
	s.NotEmpty(folderUUID)
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
	work := s.createFolder("Work", nil)
	sub := s.createFolder("Sub", &work.UUID)
	deep := s.createFolder("Deep", &sub.UUID)

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

// createFolder posts a folder via the API and returns the created item.
// parentUUID is the UUID of the parent folder, or nil for root level.
func (s *DriveItemAPITestSuite) createFolder(name string, parentUUID *string) models.DriveItem {
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

	// Assign the promoted `Model.ID` field rather than set it in the composite
	// literal: a promoted field in a struct literal needs go1.27 and this
	// module's lang is go1.25.
	var item models.DriveItem
	item.UUID = data["uuid"].(string)
	item.Name = data["name"].(string)
	// Note: ID is no longer in response, so we need to look it up from DB
	// when needed. For now, use the UUID for subsequent operations.
	_ = data["id"] // silence unused variable warning if needed
	return item
}
