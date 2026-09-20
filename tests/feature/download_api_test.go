package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type DownloadAPITestSuite struct {
	suite.Suite
	tests.TestCase
	user       models.User
	token      string
	account    models.CloudAccount
	item       models.DriveItem
	mockServer *httptest.Server
	fileData   []byte
}

func TestDownloadAPITestSuite(t *testing.T) {
	suite.Run(t, new(DownloadAPITestSuite))
}

func (s *DownloadAPITestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	s.fileData = []byte("Hello, this is the simulated file content for download testing.")

	// Mock S3 server returning file content for GET and presigned handling
	s.mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.fileData)))
			w.Header().Set("ETag", `"mock-etag-download-123"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(s.fileData)
		case http.MethodHead:
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.fileData)))
			w.Header().Set("ETag", `"mock-etag-download-123"`)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Download Test User",
		Username: "dl_" + uuid.New().String()[:8],
		Email:    "dl_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	s.token = jsonBody["data"].(map[string]any)["token"].(string)

	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "Mock S3 Download Storage",
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

	s.item = models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		Name:           "notes.txt",
		Type:           models.ItemTypeFile,
		MimeType:       "text/plain",
		Size:           int64(len(s.fileData)),
		Extension:      "txt",
		StoragePath:    fmt.Sprintf("users/%d/items/notes.txt", s.user.ID),
		ETag:           "mock-etag-download-123",
		Status:         models.ItemStatusReady,
	}
	err = facades.Orm().Query().Create(&s.item)
	s.Require().NoError(err)
}

func (s *DownloadAPITestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *DownloadAPITestSuite) TestDownloadPresignedJSON() {
	// 1. GET /v1/drive/items/{uuid}/download?mode=json
	resp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s/download?mode=json", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	data := body["data"].(map[string]any)
	downloadURL, ok := data["download_url"].(string)
	s.Require().True(ok)
	s.NotEmpty(downloadURL)
}

func (s *DownloadAPITestSuite) TestDownloadRedirect() {
	// 2. GET /v1/drive/items/{uuid}/download?mode=redirect
	resp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s/download?mode=redirect", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusFound)
	s.NotEmpty(resp.Headers().Get("Location"))
}

func (s *DownloadAPITestSuite) TestDownloadStream() {
	// 3. GET /v1/drive/items/{uuid}/download?mode=stream
	resp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/v1/drive/items/%s/download?mode=stream", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertOk()

	content, err := resp.Content()
	s.Require().NoError(err)
	s.Equal(string(s.fileData), content)
	s.Contains(resp.Headers().Get("Content-Disposition"), "notes.txt")
}
