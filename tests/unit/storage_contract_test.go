package unit

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"ponta_drive/app/contracts"
)

func TestCloudDriverInterface(t *testing.T) {
	assert.Implements(t, (*contracts.CloudDriver)(nil), &s3DriverStub{})
}

type s3DriverStub struct{}

// Implement stub methods returning nil/zero values for compile-check
func (s *s3DriverStub) Put(ctx context.Context, key string, reader io.Reader, size int64, mimeType string) error { return nil }
func (s *s3DriverStub) Get(ctx context.Context, key string) (io.ReadCloser, error) { return nil, nil }
func (s *s3DriverStub) Delete(ctx context.Context, key string) error { return nil }
func (s *s3DriverStub) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (s *s3DriverStub) GetMetadata(ctx context.Context, key string) (*contracts.ObjectMetadata, error) { return nil, nil }
func (s *s3DriverStub) GetPresignedUploadURL(ctx context.Context, key string, mimeType string, expire time.Duration) (*contracts.PresignedURLResponse, error) { return nil, nil }
func (s *s3DriverStub) GetPresignedDownloadURL(ctx context.Context, key string, fileName string, expire time.Duration) (string, error) { return "", nil }
func (s *s3DriverStub) InitiateMultipart(ctx context.Context, key string, mimeType string) (string, error) { return "", nil }
func (s *s3DriverStub) UploadPart(ctx context.Context, key string, uploadID string, partNumber int32, reader io.Reader, size int64) (string, error) { return "", nil }
func (s *s3DriverStub) CompleteMultipart(ctx context.Context, key string, uploadID string, parts []contracts.CompletedPart) error { return nil }
func (s *s3DriverStub) AbortMultipart(ctx context.Context, key string, uploadID string) error { return nil }
func (s *s3DriverStub) ListObjects(ctx context.Context, prefix string, continuationToken string, maxKeys int32) (*contracts.ListObjectsResult, error) { return nil, nil }