package services_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
	"ponta_drive/tests"
)

type CloudDriveServiceTestSuite struct {
	suite.Suite
	tests.TestCase
	service *services.CloudDriveService
	user    models.User
	account models.CloudAccount
}

func TestCloudDriveServiceTestSuite(t *testing.T) {
	suite.Run(t, new(CloudDriveServiceTestSuite))
}

func (s *CloudDriveServiceTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")
	s.service = services.NewCloudDriveService()

	s.user = models.User{
		UUID:     "user-test-uuid",
		Name:     "Test User",
		Username: "svc_user",
		Email:    "svc_user@example.com",
	}
	_ = facades.Orm().Query().Create(&s.user)

	s.account = models.CloudAccount{
		UserID:      s.user.ID,
		Name:        "Test Account",
		Provider:    models.ProviderS3,
		UsedStorage: 5000,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Bucket: "test-b", Region: "us-east-1", AccessKeyID: "k", SecretAccessKey: "s",
	})
	_ = facades.Orm().Query().Create(&s.account)
}

func (s *CloudDriveServiceTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

// TestCollectDescendants verifies that CollectDescendantItems walks the folder
// subtree via BFS, returning all items (root + descendants), all non-empty file
// storage keys, and the total size of descendant files.
func (s *CloudDriveServiceTestSuite) TestCollectDescendants() {
	ctx := context.Background()

	// Root folder
	root, err := s.service.CreateFolder(ctx, s.user.ID, s.account.ID, nil, "RootFolder")
	s.Require().NoError(err)

	// Sub folder
	sub, err := s.service.CreateFolder(ctx, s.user.ID, s.account.ID, &root.ID, "SubFolder")
	s.Require().NoError(err)

	// File inside sub
	subFile := &models.DriveItem{
		UUID:           "file-sub-uuid",
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		ParentID:       &sub.ID,
		Name:           "doc.pdf",
		Type:           models.ItemTypeFile,
		Size:           1200,
		StoragePath:    "users/1/items/file-sub-uuid-doc.pdf",
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(subFile))

	items, keys, totalSize, err := s.service.CollectDescendantItems(ctx, s.user.ID, s.account.ID, root.ID)
	s.Require().NoError(err)

	// Expect 3 items collected: root, sub, subFile
	assert.Len(s.T(), items, 3)
	assert.Equal(s.T(), []string{"users/1/items/file-sub-uuid-doc.pdf"}, keys)
	assert.Equal(s.T(), int64(1200), totalSize)
}

// TestCollectDescendantsCycleGuard verifies that a corrupted parent_id cycle
// (a folder that points back to a descendant as its parent) does not cause an
// infinite loop. The visited set must terminate the walk and return each item
// exactly once.
func (s *CloudDriveServiceTestSuite) TestCollectDescendantsCycleGuard() {
	ctx := context.Background()

	root, err := s.service.CreateFolder(ctx, s.user.ID, s.account.ID, nil, "RootFolder")
	s.Require().NoError(err)

	// Direct cycle: root's parent is itself.
	_, err = facades.Orm().Query().Model(&models.DriveItem{}).
		Where("id", root.ID).
		Update("parent_id", root.ID)
	s.Require().NoError(err)

	items, keys, totalSize, err := s.service.CollectDescendantItems(ctx, s.user.ID, s.account.ID, root.ID)
	s.Require().NoError(err)

	// Only the root should be collected; no infinite loop.
	assert.Len(s.T(), items, 1)
	assert.Empty(s.T(), keys)
	assert.Equal(s.T(), int64(0), totalSize)
}

// TestCollectDescendantsNotFound verifies that asking for a non-existent root
// returns an error rather than panicking or returning partial data.
func (s *CloudDriveServiceTestSuite) TestCollectDescendantsNotFound() {
	ctx := context.Background()

	_, _, _, err := s.service.CollectDescendantItems(ctx, s.user.ID, s.account.ID, 999999)
	s.Require().Error(err)
}

// TestCollectDescendantsEmptyRoot verifies that a leaf folder with no children
// returns just itself with no keys and zero size.
func (s *CloudDriveServiceTestSuite) TestCollectDescendantsEmptyRoot() {
	ctx := context.Background()

	root, err := s.service.CreateFolder(ctx, s.user.ID, s.account.ID, nil, "EmptyFolder")
	s.Require().NoError(err)

	items, keys, totalSize, err := s.service.CollectDescendantItems(ctx, s.user.ID, s.account.ID, root.ID)
	s.Require().NoError(err)

	assert.Len(s.T(), items, 1)
	assert.Empty(s.T(), keys)
	assert.Equal(s.T(), int64(0), totalSize)
}
