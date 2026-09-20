package storage

import (
	"context"
	"fmt"
	"sync"

	"ponta_drive/app/contracts"
	"ponta_drive/app/models"
)

// DriverFactory creates and caches CloudDriver instances based on account credentials.
type DriverFactory struct {
	mu    sync.RWMutex
	cache map[uint]contracts.CloudDriver
}

// NewDriverFactory initializes a new DriverFactory instance.
func NewDriverFactory() *DriverFactory {
	return &DriverFactory{
		cache: make(map[uint]contracts.CloudDriver),
	}
}

// BuildDriver constructs a fresh CloudDriver for the given provider and credentials.
func (f *DriverFactory) BuildDriver(ctx context.Context, provider string, creds *models.S3Credentials) (contracts.CloudDriver, error) {
	if creds == nil {
		return nil, fmt.Errorf("credentials cannot be nil")
	}

	switch provider {
	case models.ProviderS3, models.ProviderCloudflareR2, models.ProviderMinIO:
		cfg := ConfigFromS3Credentials(creds)
		return NewS3Driver(ctx, cfg)

	case models.ProviderGoogleDrive, models.ProviderOneDrive:
		return nil, fmt.Errorf("provider %q is not yet supported", provider)

	default:
		return nil, fmt.Errorf("unrecognized cloud provider: %q", provider)
	}
}

// DriverForAccount resolves a cached CloudDriver or initializes a new one for the CloudAccount.
func (f *DriverFactory) DriverForAccount(ctx context.Context, account *models.CloudAccount) (contracts.CloudDriver, error) {
	if account == nil {
		return nil, fmt.Errorf("cloud account cannot be nil")
	}

	// Check cache if account is already persisted (ID > 0)
	if account.ID > 0 {
		f.mu.RLock()
		if driver, found := f.cache[account.ID]; found {
			f.mu.RUnlock()
			return driver, nil
		}
		f.mu.RUnlock()
	}

	creds, err := account.GetCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials for account %d: %w", account.ID, err)
	}

	driver, err := f.BuildDriver(ctx, account.Provider, creds)
	if err != nil {
		return nil, err
	}

	if account.ID > 0 {
		f.mu.Lock()
		f.cache[account.ID] = driver
		f.mu.Unlock()
	}

	return driver, nil
}

// InvalidateCache evicts the cached driver for an account when updated or deleted.
func (f *DriverFactory) InvalidateCache(accountID uint) {
	f.mu.Lock()
	delete(f.cache, accountID)
	f.mu.Unlock()
}
