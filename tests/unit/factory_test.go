package unit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ponta_drive/app/models"
	"ponta_drive/app/services/storage"
	_ "ponta_drive/tests"
)

func TestDriverFactoryBuildDriver(t *testing.T) {
	factory := storage.NewDriverFactory()
	ctx := context.Background()

	creds := &models.S3Credentials{
		Endpoint:        "https://s3.us-east-1.amazonaws.com",
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA...",
		SecretAccessKey: "secret...",
	}

	// 1. S3 Provider
	driver, err := factory.BuildDriver(ctx, models.ProviderS3, creds)
	require.NoError(t, err)
	assert.NotNil(t, driver)

	// 2. Cloudflare R2 Provider
	r2Creds := &models.S3Credentials{
		Endpoint:        "https://account-id.r2.cloudflarestorage.com",
		Bucket:          "r2-bucket",
		Region:          "auto",
		AccessKeyID:     "r2-key",
		SecretAccessKey: "r2-secret",
	}
	r2Driver, err := factory.BuildDriver(ctx, models.ProviderCloudflareR2, r2Creds)
	require.NoError(t, err)
	assert.NotNil(t, r2Driver)

	// 3. MinIO Provider
	minioCreds := &models.S3Credentials{
		Endpoint:        "http://127.0.0.1:9000",
		Bucket:          "minio-bucket",
		AccessKeyID:     "minio-admin",
		SecretAccessKey: "minio-password",
		UsePathStyle:    true,
	}
	minioDriver, err := factory.BuildDriver(ctx, models.ProviderMinIO, minioCreds)
	require.NoError(t, err)
	assert.NotNil(t, minioDriver)

	// 4. Unsupported Provider (Google Drive / OneDrive)
	_, err = factory.BuildDriver(ctx, models.ProviderGoogleDrive, creds)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet supported")

	_, err = factory.BuildDriver(ctx, "unsupported_provider", creds)
	assert.Error(t, err)
}

func TestDriverFactoryDriverForAccount(t *testing.T) {
	factory := storage.NewDriverFactory()
	ctx := context.Background()

	account := &models.CloudAccount{
		UserID:   1,
		Name:     "R2 Account",
		Provider: models.ProviderCloudflareR2,
	}
	account.ID = 100

	creds := &models.S3Credentials{
		Endpoint:        "https://account-id.r2.cloudflarestorage.com",
		Bucket:          "my-r2",
		Region:          "auto",
		AccessKeyID:     "r2-key",
		SecretAccessKey: "r2-secret",
	}
	err := account.SetCredentials(creds)
	require.NoError(t, err)

	// 1. Get driver for account
	driver1, err := factory.DriverForAccount(ctx, account)
	require.NoError(t, err)
	assert.NotNil(t, driver1)

	// 2. Cache hit check: same account ID should return cached instance
	driver2, err := factory.DriverForAccount(ctx, account)
	require.NoError(t, err)
	assert.Same(t, driver1, driver2, "Repeated calls for same account should reuse cached driver")

	// 3. Invalidate cache
	factory.InvalidateCache(account.ID)
	driver3, err := factory.DriverForAccount(ctx, account)
	require.NoError(t, err)
	assert.NotSame(t, driver1, driver3, "After invalidation, a new driver should be created")
}
