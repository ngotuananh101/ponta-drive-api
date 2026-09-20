package feature

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/requests"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type CloudAccountTestSuite struct {
	suite.Suite
	tests.TestCase
	user models.User
}

func TestCloudAccountTestSuite(t *testing.T) {
	suite.Run(t, new(CloudAccountTestSuite))
}

func (s *CloudAccountTestSuite) SetupTest() {
	// Run migrations to ensure cloud_accounts table exists
	_ = facades.Artisan().Call("migrate")

	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Cloud Test User",
		Username: "clouduser_" + uuid.New().String()[:8],
		Email:    "cloud_" + uuid.New().String()[:8] + "@example.com",
		Password: "hashed_password",
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)
}

func (s *CloudAccountTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *CloudAccountTestSuite) TestSchemaHasCloudAccountsTable() {
	s.True(facades.Schema().HasTable("cloud_accounts"))
}

func (s *CloudAccountTestSuite) TestModelCredentialsEncryptionRoundTrip() {
	account := models.CloudAccount{
		UserID:       s.user.ID,
		Name:         "My AWS Bucket",
		Provider:     models.ProviderS3,
		IsDefault:    true,
		IsActive:     true,
		TotalStorage: 100 * 1024 * 1024 * 1024,
	}

	creds := models.S3Credentials{
		Endpoint:        "https://s3.us-east-1.amazonaws.com",
		Bucket:          "test-bucket-ponta",
		Region:          "us-east-1",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		UsePathStyle:    false,
		PublicURL:       "https://cdn.example.com",
	}

	err := account.SetCredentials(&creds)
	s.Require().NoError(err)
	s.NotEmpty(account.Credentials)
	s.NotContains(account.Credentials, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	// Persist to DB
	err = facades.Orm().Query().Create(&account)
	s.Require().NoError(err)
	s.Greater(account.ID, uint(0))

	// Fetch from DB
	var fetched models.CloudAccount
	err = facades.Orm().Query().Where("id", account.ID).First(&fetched)
	s.Require().NoError(err)
	s.Equal(account.Name, fetched.Name)
	s.Equal(account.Provider, fetched.Provider)
	s.True(fetched.IsDefault)

	// Decrypt
	decryptedCreds, err := fetched.GetCredentials()
	s.Require().NoError(err)
	s.Require().NotNil(decryptedCreds)
	s.Equal("https://s3.us-east-1.amazonaws.com", decryptedCreds.Endpoint)
	s.Equal("test-bucket-ponta", decryptedCreds.Bucket)
	s.Equal("us-east-1", decryptedCreds.Region)
	s.Equal("AKIAIOSFODNN7EXAMPLE", decryptedCreds.AccessKeyID)
	s.Equal("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", decryptedCreds.SecretAccessKey)
	s.False(decryptedCreds.UsePathStyle)
	s.Equal("https://cdn.example.com", decryptedCreds.PublicURL)

	// ToResponse check: SecretAccessKey must not leak
	resp := fetched.ToResponse()
	s.Equal(fetched.ID, resp["id"])
	s.Equal(fetched.Name, resp["name"])
	safeCreds, ok := resp["credentials"].(map[string]any)
	s.Require().True(ok)
	s.Equal("test-bucket-ponta", safeCreds["bucket"])
	s.NotContains(safeCreds, "secret_access_key")
}

func (s *CloudAccountTestSuite) TestValidationDTOs() {
	ctx := context.Background()

	// 1. StoreCloudAccountRequest validation
	validator, err := facades.Validation().Make(ctx, map[string]any{
		"name":              "Primary S3",
		"provider":          models.ProviderS3,
		"bucket":            "ponta-bucket",
		"region":            "us-east-1",
		"access_key_id":     "AKIA...",
		"secret_access_key": "secret...",
	}, (&requests.StoreCloudAccountRequest{}).Rules(nil))
	s.Require().NoError(err)
	s.False(validator.Fails(), "Valid store payload should pass validation")

	// Missing required fields
	invalidValidator, err := facades.Validation().Make(ctx, map[string]any{
		"name": "Missing Provider and Bucket",
	}, (&requests.StoreCloudAccountRequest{}).Rules(nil))
	s.Require().NoError(err)
	s.True(invalidValidator.Fails(), "Payload missing required fields should fail")
	s.Contains(invalidValidator.Errors().All(), "provider")
	s.Contains(invalidValidator.Errors().All(), "bucket")

	// 2. TestCloudAccountRequest validation
	testValidator, err := facades.Validation().Make(ctx, map[string]any{
		"provider":          models.ProviderCloudflareR2,
		"bucket":            "r2-bucket",
		"region":            "auto",
		"access_key_id":     "r2-key",
		"secret_access_key": "r2-secret",
	}, (&requests.TestCloudAccountRequest{}).Rules(nil))
	s.Require().NoError(err)
	s.False(testValidator.Fails(), "Valid test payload should pass validation")

	// 3. UpdateCloudAccountRequest validation
	updateValidator, err := facades.Validation().Make(ctx, map[string]any{
		"name": "Updated S3 Account",
	}, (&requests.UpdateCloudAccountRequest{}).Rules(nil))
	s.Require().NoError(err)
	s.False(updateValidator.Fails(), "Valid update payload should pass validation")
}
