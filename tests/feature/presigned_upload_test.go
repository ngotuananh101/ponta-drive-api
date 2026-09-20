package feature

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type PresignedUploadTestSuite struct {
	suite.Suite
	tests.TestCase
	user       models.User
	token      string
	account    models.CloudAccount
	mockServer *httptest.Server
}

func TestPresignedUploadTestSuite(t *testing.T) {
	suite.Run(t, new(PresignedUploadTestSuite))
}

func (s *PresignedUploadTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	// Start mock S3 server
	s.mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.Header().Set("ETag", `"mock-etag-abc"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			w.Header().Set("Content-Length", "1048576")
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("ETag", `"mock-etag-abc"`)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Presign User",
		Username: "presign_" + uuid.New().String()[:8],
		Email:    "presign_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	// Obtain JWT
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	dataMap := jsonBody["data"].(map[string]any)
	s.token = dataMap["token"].(string)

	// Create CloudAccount with mock endpoint
	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "Mock S3 Storage",
		Provider:  models.ProviderMinIO,
		IsDefault: true,
		IsActive:  true,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Endpoint:        s.mockServer.URL,
		Bucket:          "mock-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "mock-key",
		SecretAccessKey: "mock-secret",
		UsePathStyle:    true,
	})
	err = facades.Orm().Query().Create(&s.account)
	s.Require().NoError(err)
}

func (s *PresignedUploadTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *PresignedUploadTestSuite) TestPresignedUploadFlow() {
	// 1. Initiate Presigned Upload (POST /api/v1/drive/upload/presigned)
	initPayload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"file_name":        "tutorial.mp4",
		"size":             1048576,
		"mime_type":        "video/mp4",
	})

	initResp, err := s.Http(s.T()).WithToken(s.token).Post("/api/v1/drive/upload/presigned", bytes.NewBuffer(initPayload))
	s.Require().NoError(err)
	initResp.AssertStatus(http.StatusOK)

	initBody, err := initResp.Json()
	s.Require().NoError(err)
	data := initBody["data"].(map[string]any)
	itemUUID, ok := data["item_uuid"].(string)
	s.Require().True(ok)
	s.NotEmpty(itemUUID)
	uploadURL, ok := data["upload_url"].(string)
	s.Require().True(ok)
	s.NotEmpty(uploadURL)
	s.Equal("PUT", data["method"])

	// Verify item exists with status 'uploading'
	var item models.DriveItem
	err = facades.Orm().Query().Where("uuid", itemUUID).First(&item)
	s.Require().NoError(err)
	s.Equal(models.ItemStatusUploading, item.Status)
	s.Equal("mp4", item.Extension)
	s.Equal("tutorial.mp4", item.Name)

	// 2. Complete Presigned Upload (POST /api/v1/drive/upload/presigned/complete)
	completePayload, _ := json.Marshal(map[string]any{
		"item_uuid": itemUUID,
	})

	compResp, err := s.Http(s.T()).WithToken(s.token).Post("/api/v1/drive/upload/presigned/complete", bytes.NewBuffer(completePayload))
	s.Require().NoError(err)
	compResp.AssertStatus(http.StatusOK)

	compBody, err := compResp.Json()
	s.Require().NoError(err)
	compData := compBody["data"].(map[string]any)
	s.Equal(models.ItemStatusReady, compData["status"])
	s.Equal("mock-etag-abc", compData["etag"])

	// Verify item in DB is 'ready'
	err = facades.Orm().Query().Where("uuid", itemUUID).First(&item)
	s.Require().NoError(err)
	s.Equal(models.ItemStatusReady, item.Status)
	s.Equal("mock-etag-abc", item.ETag)
}
