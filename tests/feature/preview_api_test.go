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

type PreviewAPITestSuite struct {
	suite.Suite
	tests.TestCase
	user       models.User
	token      string
	account    models.CloudAccount
	item       models.DriveItem
	mockServer *httptest.Server
	fileData   []byte
}

func TestPreviewAPITestSuite(t *testing.T) {
	suite.Run(t, new(PreviewAPITestSuite))
}

func (s *PreviewAPITestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	s.fileData = []byte("0123456789")

	// One range-aware mock for the whole suite. It must NOT be replaced per
	// test: CloudDriveService caches its driver by account id for the process
	// lifetime, so a swapped endpoint would be ignored and the test would hit
	// a closed server.
	s.mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Accept-Ranges", "bytes")
		if rng := r.Header.Get("Range"); rng != "" {
			// The suite only ever requests "bytes=2-5".
			w.Header().Set("Content-Range", "bytes 2-5/10")
			w.Header().Set("Content-Length", "4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(s.fileData[2:6])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.fileData)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.fileData)
	}))

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID: uuid.New().String(), Name: "Preview User",
		Username: "pv_" + uuid.New().String()[:8],
		Email:    "pv_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	s.Require().NoError(facades.Orm().Query().Create(&s.user))

	loginPayload, _ := json.Marshal(map[string]string{"email": s.user.Email, "password": "password123"})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()
	body, _ := resp.Json()
	s.token = body["data"].(map[string]any)["token"].(string)

	s.account = models.CloudAccount{
		UserID: s.user.ID, Name: "Preview Storage",
		Provider: models.ProviderMinIO, IsDefault: true, IsActive: true,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Endpoint: s.mockServer.URL, Bucket: "mock-bucket", Region: "us-east-1",
		AccessKeyID: "mock-key", SecretAccessKey: "mock-secret", UsePathStyle: true,
	})
	s.Require().NoError(facades.Orm().Query().Create(&s.account))

	s.item = models.DriveItem{
		UUID: uuid.New().String(), UserID: s.user.ID, CloudAccountID: s.account.ID,
		Name: "photo.png", Type: models.ItemTypeFile, MimeType: "image/png",
		Size: int64(len(s.fileData)), Extension: "png",
		StoragePath: fmt.Sprintf("users/%d/items/photo.png", s.user.ID),
		Status:      models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&s.item))
}

func (s *PreviewAPITestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *PreviewAPITestSuite) TestPreviewMediaIsDirect() {
	resp, err := s.Http(s.T()).WithToken(s.token).Get(
		fmt.Sprintf("/v1/drive/items/%s/preview?mode=json", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertOk()

	body, _ := resp.Json()
	data := body["data"].(map[string]any)
	s.Equal("direct", data["strategy"])
	s.NotEmpty(data["url"])
	s.NotEmpty(data["download_url"])
}

func (s *PreviewAPITestSuite) TestPreviewLargeFetchBasedIsFallback() {
	// A fetch-based file over the cap with no public URL cannot be proxied.
	s.item.MimeType = "application/pdf"
	s.item.Extension = "pdf"
	s.item.Size = 51 * 1024 * 1024
	s.Require().NoError(facades.Orm().Query().Save(&s.item))

	resp, err := s.Http(s.T()).WithToken(s.token).Get(
		fmt.Sprintf("/v1/drive/items/%s/preview?mode=json", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertOk()

	body, _ := resp.Json()
	data := body["data"].(map[string]any)
	s.Equal("fallback", data["strategy"])
	s.Equal("too_large", data["reason"])
	s.NotEmpty(data["download_url"])
}

func (s *PreviewAPITestSuite) TestPreviewFolderRejected() {
	folder := models.DriveItem{
		UUID: uuid.New().String(), UserID: s.user.ID, CloudAccountID: s.account.ID,
		Name: "docs", Type: models.ItemTypeFolder, Status: models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&folder))

	resp, err := s.Http(s.T()).WithToken(s.token).Get(
		fmt.Sprintf("/v1/drive/items/%s/preview?mode=json", folder.UUID))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusBadRequest)
}

func (s *PreviewAPITestSuite) TestContentStreamsWithinCap() {
	// Size matches the mock's body (see SetupTest); the handler echoes it as
	// Content-Length, so a mismatch would truncate the read.
	resp, err := s.Http(s.T()).WithToken(s.token).Get(
		fmt.Sprintf("/v1/drive/items/%s/content", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertOk()
	s.Equal("inline", resp.Headers().Get("Content-Disposition"))
	content, _ := resp.Content()
	s.Equal(string(s.fileData), content)
}

func (s *PreviewAPITestSuite) TestContentRejectsOverCap() {
	s.item.Size = 51 * 1024 * 1024
	s.Require().NoError(facades.Orm().Query().Save(&s.item))

	resp, err := s.Http(s.T()).WithToken(s.token).Get(
		fmt.Sprintf("/v1/drive/items/%s/content", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusRequestEntityTooLarge)
}

func (s *PreviewAPITestSuite) TestContentServesRange() {
	// s.item.Size is 10 (see SetupTest), so bytes=2-5 is satisfiable.
	req := s.Http(s.T()).WithToken(s.token).WithHeader("Range", "bytes=2-5")
	resp, err := req.Get(fmt.Sprintf("/v1/drive/items/%s/content", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusPartialContent)
	s.Equal("bytes 2-5/10", resp.Headers().Get("Content-Range"))
	content, _ := resp.Content()
	s.Equal("2345", content)
}

func (s *PreviewAPITestSuite) TestContentRejectsBadRange() {
	req := s.Http(s.T()).WithToken(s.token).WithHeader("Range", "bytes=200-300")
	resp, err := req.Get(fmt.Sprintf("/v1/drive/items/%s/content", s.item.UUID))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusRequestedRangeNotSatisfiable)
}

