package feature

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/goravel/framework/support/carbon"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/contracts"
	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type DriveItemAndSessionTestSuite struct {
	suite.Suite
	tests.TestCase
	user    models.User
	account models.CloudAccount
}

func TestDriveItemAndSessionTestSuite(t *testing.T) {
	suite.Run(t, new(DriveItemAndSessionTestSuite))
}

func (s *DriveItemAndSessionTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Drive User",
		Username: "drive_" + uuid.New().String()[:8],
		Email:    "drive_" + uuid.New().String()[:8] + "@example.com",
		Password: "password",
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	s.account = models.CloudAccount{
		UserID:   s.user.ID,
		Name:     "Drive Account",
		Provider: models.ProviderS3,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	})
	err = facades.Orm().Query().Create(&s.account)
	s.Require().NoError(err)
}

func (s *DriveItemAndSessionTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.UploadSession{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *DriveItemAndSessionTestSuite) TestDriveItemCRUDAndHierarchy() {
	// 1. Create a root folder
	folder := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		ParentID:       nil,
		Name:           "Documents",
		Type:           models.ItemTypeFolder,
		Status:         models.ItemStatusReady,
	}
	err := facades.Orm().Query().Create(&folder)
	s.Require().NoError(err)
	s.Greater(folder.ID, uint(0))

	// 2. Create a child file inside the folder
	parentID := folder.ID
	file := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		ParentID:       &parentID,
		Name:           "report.pdf",
		Type:           models.ItemTypeFile,
		MimeType:       "application/pdf",
		Size:           1048576,
		Extension:      "pdf",
		StoragePath:    "users/1/items/uuid-file.pdf",
		ETag:           "etag-hash-123",
		IsStarred:      true,
		Status:         models.ItemStatusReady,
	}
	err = facades.Orm().Query().Create(&file)
	s.Require().NoError(err)
	s.Greater(file.ID, uint(0))

	// 3. Query child items
	var children []models.DriveItem
	err = facades.Orm().Query().Where("parent_id", folder.ID).Find(&children)
	s.Require().NoError(err)
	s.Len(children, 1)
	s.Equal("report.pdf", children[0].Name)
	s.True(children[0].IsStarred)
	s.Equal(int64(1048576), children[0].Size)

	// 4. Test ToResponse
	resp := file.ToResponse()
	s.Equal(file.UUID, resp["uuid"])
	s.Equal(file.Name, resp["name"])
	s.Equal("pdf", resp["extension"])
}

func (s *DriveItemAndSessionTestSuite) TestUploadSessionPartsRoundTrip() {
	exp := carbon.NewDateTime(carbon.New(time.Now().Add(24 * time.Hour)))
	session := models.UploadSession{
		SessionID:      uuid.New().String(),
		UploadID:       "s3-multipart-id-999",
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		FileName:       "big-backup.zip",
		MimeType:       "application/zip",
		TotalSize:      50 * 1024 * 1024,
		ChunkSize:      5 * 1024 * 1024,
		TotalParts:     10,
		StoragePath:    "uploads/big-backup.zip",
		ExpiresAt:      exp,
	}

	// Add parts
	err := session.AddUploadedPart(contracts.CompletedPart{PartNumber: 1, ETag: "etag-part-1"})
	s.Require().NoError(err)
	err = session.AddUploadedPart(contracts.CompletedPart{PartNumber: 2, ETag: "etag-part-2"})
	s.Require().NoError(err)

	err = facades.Orm().Query().Create(&session)
	s.Require().NoError(err)
	s.Greater(session.ID, uint(0))

	// Fetch back
	var fetched models.UploadSession
	err = facades.Orm().Query().Where("id", session.ID).First(&fetched)
	s.Require().NoError(err)

	parts, err := fetched.GetUploadedParts()
	s.Require().NoError(err)
	s.Len(parts, 2)
	s.Equal(int32(1), parts[0].PartNumber)
	s.Equal("etag-part-1", parts[0].ETag)
	s.Equal(int32(2), parts[1].PartNumber)
	s.Equal("etag-part-2", parts[1].ETag)
}
