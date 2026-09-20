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

type BucketSyncTestSuite struct {
	suite.Suite
	tests.TestCase
	user       models.User
	token      string
	account    models.CloudAccount
	mockServer *httptest.Server
}

func TestBucketSyncTestSuite(t *testing.T) {
	suite.Run(t, new(BucketSyncTestSuite))
}

func (s *BucketSyncTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	// Start mock S3 server returning ListObjectsV2 XML response
	s.mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>mock-bucket</Name>
  <Prefix></Prefix>
  <KeyCount>2</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>photos/vacation/beach.jpg</Key>
    <LastModified>2026-09-20T10:00:00.000Z</LastModified>
    <ETag>"mock-etag-beach"</ETag>
    <Size>2048576</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
  <Contents>
    <Key>readme.txt</Key>
    <LastModified>2026-09-20T10:00:00.000Z</LastModified>
    <ETag>"mock-etag-readme"</ETag>
    <Size>1024</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`))
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Sync Test User",
		Username: "sync_" + uuid.New().String()[:8],
		Email:    "sync_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	s.token = jsonBody["data"].(map[string]any)["token"].(string)

	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "Mock Sync Storage",
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

func (s *BucketSyncTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *BucketSyncTestSuite) TestBucketSyncWorkflow() {
	// 1. Trigger sync via POST /api/v1/cloud-accounts/{id}/sync
	syncResp, err := s.Http(s.T()).WithToken(s.token).Post(fmt.Sprintf("/api/v1/cloud-accounts/%d/sync", s.account.ID), nil)
	s.Require().NoError(err)
	syncResp.AssertOk()

	syncBody, err := syncResp.Json()
	s.Require().NoError(err)
	s.Equal("ok", syncBody["status"])

	// 2. Verify root folder contains 'readme.txt' and 'photos'
	var rootItems []models.DriveItem
	err = facades.Orm().Query().
		Where("user_id", s.user.ID).
		Where("cloud_account_id", s.account.ID).
		Where("parent_id IS NULL").
		Find(&rootItems)
	s.Require().NoError(err)
	s.Len(rootItems, 2)

	var photosFolder *models.DriveItem
	var readmeFile *models.DriveItem
	for i := range rootItems {
		if rootItems[i].Name == "photos" {
			photosFolder = &rootItems[i]
		} else if rootItems[i].Name == "readme.txt" {
			readmeFile = &rootItems[i]
		}
	}

	s.Require().NotNil(photosFolder)
	s.Equal(models.ItemTypeFolder, photosFolder.Type)

	s.Require().NotNil(readmeFile)
	s.Equal(models.ItemTypeFile, readmeFile.Type)
	s.Equal(int64(1024), readmeFile.Size)
	s.Equal("mock-etag-readme", readmeFile.ETag)

	// 3. Verify 'photos' contains 'vacation'
	var photosChildren []models.DriveItem
	err = facades.Orm().Query().
		Where("user_id", s.user.ID).
		Where("parent_id", photosFolder.ID).
		Find(&photosChildren)
	s.Require().NoError(err)
	s.Len(photosChildren, 1)
	s.Equal("vacation", photosChildren[0].Name)
	s.Equal(models.ItemTypeFolder, photosChildren[0].Type)

	// 4. Verify 'vacation' contains 'beach.jpg'
	var vacationChildren []models.DriveItem
	err = facades.Orm().Query().
		Where("user_id", s.user.ID).
		Where("parent_id", photosChildren[0].ID).
		Find(&vacationChildren)
	s.Require().NoError(err)
	s.Len(vacationChildren, 1)
	s.Equal("beach.jpg", vacationChildren[0].Name)
	s.Equal(models.ItemTypeFile, vacationChildren[0].Type)
	s.Equal(int64(2048576), vacationChildren[0].Size)
	s.Equal("mock-etag-beach", vacationChildren[0].ETag)
	s.Equal("jpg", vacationChildren[0].Extension)

	// 5. Test idempotency: re-running sync should not create duplicate items
	syncResp2, err := s.Http(s.T()).WithToken(s.token).Post(fmt.Sprintf("/api/v1/cloud-accounts/%d/sync", s.account.ID), nil)
	s.Require().NoError(err)
	syncResp2.AssertOk()

	totalItems, err := facades.Orm().Query().Model(&models.DriveItem{}).
		Where("user_id", s.user.ID).
		Where("cloud_account_id", s.account.ID).
		Count()
	s.Require().NoError(err)
	s.Equal(int64(4), totalItems) // photos, vacation, beach.jpg, readme.txt
}
