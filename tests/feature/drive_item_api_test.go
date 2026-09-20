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
	folderID := uint(folderData["id"].(float64))
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

	// 3. Create a Child Item inside the folder
	childFolderPayload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"parent_id":        folderID,
		"name":             "Sub Projects",
	})
	childResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/items/folders", bytes.NewBuffer(childFolderPayload))
	s.Require().NoError(err)
	childResp.AssertStatus(http.StatusCreated)

	childBody, _ := childResp.Json()
	childUUID := childBody["data"].(map[string]any)["uuid"].(string)

	// 4. List items inside folder (GET /v1/drive/items?cloud_account_id=...&parent_id=...)
	folderItemsResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items?cloud_account_id=%d&parent_id=%d", s.account.ID, folderID))
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
