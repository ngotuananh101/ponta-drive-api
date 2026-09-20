// Package contracts defines interfaces for cloud storage drivers.
package contracts

import (
	"context"
	"io"
	"time"
)

type ObjectMetadata struct {
	Key          string
	Size         int64
	MimeType     string
	ETag         string
	LastModified time.Time
}

type PresignedURLResponse struct {
	URL     string
	Method  string
	Headers map[string]string
	Expires time.Time
}

type CompletedPart struct {
	PartNumber int32
	ETag       string
}

type ListObjectsResult struct {
	Objects          []ObjectMetadata
	CommonPrefixes   []string
	NextContinuation string
	IsTruncated      bool
}

type CloudDriver interface {
	Put(ctx context.Context, key string, reader io.Reader, size int64, mimeType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	GetMetadata(ctx context.Context, key string) (*ObjectMetadata, error)
	GetPresignedUploadURL(ctx context.Context, key string, mimeType string, expire time.Duration) (*PresignedURLResponse, error)
	GetPresignedDownloadURL(ctx context.Context, key string, fileName string, expire time.Duration) (string, error)
	InitiateMultipart(ctx context.Context, key string, mimeType string) (string, error)
	UploadPart(ctx context.Context, key string, uploadID string, partNumber int32, reader io.Reader, size int64) (string, error)
	CompleteMultipart(ctx context.Context, key string, uploadID string, parts []CompletedPart) error
	AbortMultipart(ctx context.Context, key string, uploadID string) error
	ListObjects(ctx context.Context, prefix string, continuationToken string, maxKeys int32) (*ListObjectsResult, error)
}