package unit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ponta_drive/app/contracts"
	"ponta_drive/app/services/storage"
)

func TestS3DriverImplementsInterface(t *testing.T) {
	var _ contracts.CloudDriver = (*storage.S3Driver)(nil)
}

func TestS3DriverConfig(t *testing.T) {
	cfg := storage.S3Config{
		Bucket:          "my-test-bucket",
		Region:          "ap-southeast-1",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Endpoint:        "http://localhost:9000",
		UsePathStyle:    true,
	}

	driver, err := storage.NewS3Driver(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, driver)
	assert.Equal(t, "my-test-bucket", driver.GetBucket())
	assert.Equal(t, "ap-southeast-1", driver.GetRegion())
}

func TestS3DriverDefaultRegion(t *testing.T) {
	cfg := storage.S3Config{
		Bucket:          "my-test-bucket",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
	}

	driver, err := storage.NewS3Driver(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", driver.GetRegion(), "Empty region should default to us-east-1")
}

func TestS3DriverPresignedURLs(t *testing.T) {
	cfg := storage.S3Config{
		Bucket:          "my-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	driver, err := storage.NewS3Driver(context.Background(), cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Presigned Upload
	uploadResp, err := driver.GetPresignedUploadURL(ctx, "uploads/file.png", "image/png", 15*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, uploadResp.URL)
	assert.Equal(t, "PUT", uploadResp.Method)
	assert.Contains(t, uploadResp.URL, "uploads/file.png")

	// Presigned Download
	downloadURL, err := driver.GetPresignedDownloadURL(ctx, "uploads/file.png", "downloaded.png", 15*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, downloadURL)
	assert.Contains(t, downloadURL, "uploads/file.png")
	assert.Contains(t, downloadURL, "response-content-disposition")
}

func TestS3DriverOperationsWithMockServer(t *testing.T) {
	// Mock S3 HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.Query().Has("uploadId") && r.URL.Query().Has("partNumber") {
				// UploadPart
				w.Header().Set("ETag", `"etag-part-1"`)
				w.WriteHeader(http.StatusOK)
				return
			}
			// PutObject
			w.Header().Set("ETag", `"test-etag-123"`)
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			if r.URL.Path == "/test-bucket" || r.URL.Path == "/" {
				// ListObjectsV2
				w.Header().Set("Content-Type", "application/xml")
				xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Name>test-bucket</Name>
    <Prefix></Prefix>
    <KeyCount>1</KeyCount>
    <MaxKeys>1000</MaxKeys>
    <IsTruncated>false</IsTruncated>
    <Contents>
        <Key>test.txt</Key>
        <LastModified>2026-09-20T00:00:00.000Z</LastModified>
        <ETag>&quot;etag-123&quot;</ETag>
        <Size>12</Size>
        <StorageClass>STANDARD</StorageClass>
    </Contents>
</ListBucketResult>`
				_, _ = w.Write([]byte(xmlResp))
				return
			}
			// GetObject
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", `"test-etag-123"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Hello S3"))

		case http.MethodHead:
			if r.URL.Path == "/test-bucket/non-existent.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", "12")
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", `"test-etag-123"`)
			w.WriteHeader(http.StatusOK)

		case http.MethodDelete:
			if r.URL.Query().Has("uploadId") {
				// AbortMultipart
				w.WriteHeader(http.StatusNoContent)
				return
			}
			// DeleteObject
			w.WriteHeader(http.StatusNoContent)

		case http.MethodPost:
			if r.URL.Query().Has("uploads") {
				// InitiateMultipart
				w.Header().Set("Content-Type", "application/xml")
				xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Bucket>test-bucket</Bucket>
    <Key>large-file.bin</Key>
    <UploadId>upload-id-999</UploadId>
</InitiateMultipartUploadResult>`
				_, _ = w.Write([]byte(xmlResp))
				return
			}
			if r.URL.Query().Has("uploadId") {
				// CompleteMultipart
				w.Header().Set("Content-Type", "application/xml")
				xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Bucket>test-bucket</Bucket>
    <Key>large-file.bin</Key>
    <ETag>&quot;final-multipart-etag&quot;</ETag>
</CompleteMultipartUploadResult>`
				_, _ = w.Write([]byte(xmlResp))
				return
			}
		}
	}))
	defer mockServer.Close()

	cfg := storage.S3Config{
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		Endpoint:        mockServer.URL,
		UsePathStyle:    true,
	}

	driver, err := storage.NewS3Driver(context.Background(), cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// 1. Put
	err = driver.Put(ctx, "hello.txt", bytes.NewReader([]byte("Hello S3")), 8, "text/plain")
	assert.NoError(t, err)

	// 2. Get
	body, err := driver.Get(ctx, "hello.txt")
	require.NoError(t, err)
	defer body.Close()
	content, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "Hello S3", string(content))

	// 3. Exists
	exists, err := driver.Exists(ctx, "hello.txt")
	assert.NoError(t, err)
	assert.True(t, exists)

	notExists, err := driver.Exists(ctx, "non-existent.txt")
	assert.NoError(t, err)
	assert.False(t, notExists)

	// 4. GetMetadata
	meta, err := driver.GetMetadata(ctx, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(12), meta.Size)
	assert.Equal(t, "text/plain", meta.MimeType)

	// 5. Delete
	err = driver.Delete(ctx, "hello.txt")
	assert.NoError(t, err)

	// 6. Multipart flow
	uploadID, err := driver.InitiateMultipart(ctx, "large-file.bin", "application/octet-stream")
	require.NoError(t, err)
	assert.Equal(t, "upload-id-999", uploadID)

	partETag, err := driver.UploadPart(ctx, "large-file.bin", uploadID, 1, bytes.NewReader([]byte("part-1")), 6)
	require.NoError(t, err)
	assert.NotEmpty(t, partETag)

	err = driver.CompleteMultipart(ctx, "large-file.bin", uploadID, []contracts.CompletedPart{
		{PartNumber: 1, ETag: partETag},
	})
	assert.NoError(t, err)

	err = driver.AbortMultipart(ctx, "large-file.bin", uploadID)
	assert.NoError(t, err)

	// 7. ListObjects
	listRes, err := driver.ListObjects(ctx, "", "", 100)
	require.NoError(t, err)
	require.Len(t, listRes.Objects, 1)
	assert.Equal(t, "test.txt", listRes.Objects[0].Key)
	assert.Equal(t, int64(12), listRes.Objects[0].Size)
}

// newDeleteObjectsTestDriver sets up a mock S3 HTTP server that counts
// DeleteObjects calls and a configured S3Driver pointing at it.
// Tests using this helper reset the counter between sub-cases via the
// returned pointer.
func newDeleteObjectsTestDriver(t *testing.T) (*storage.S3Driver, *int32) {
	t.Helper()
	var calls int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Query().Has("delete") {
			atomic.AddInt32(&calls, 1)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Deleted><Key>file1.txt</Key></Deleted>
</DeleteResult>`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(mockServer.Close)

	cfg := storage.S3Config{
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		Endpoint:        mockServer.URL,
		UsePathStyle:    true,
	}

	driver, err := storage.NewS3Driver(context.Background(), cfg)
	require.NoError(t, err)
	return driver, &calls
}

// TestS3DriverDeleteObjectsChunkingAndEmpty asserts the DeleteObjects
// chunking behaviour across three sub-cases:
//   - 0 keys -> 0 DeleteObjects calls
//   - 2 keys -> 1 call
//   - 1001 keys -> 2 calls
func TestS3DriverDeleteObjectsChunkingAndEmpty(t *testing.T) {
	driver, deleteObjectsCalls := newDeleteObjectsTestDriver(t)
	ctx := context.Background()

	// 1. Empty keys slice should be a no-op
	*deleteObjectsCalls = 0
	err := driver.DeleteObjects(ctx, []string{})
	assert.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(deleteObjectsCalls),
		"empty key set must not issue any DeleteObjects call")

	// 2. Non-empty keys slice
	*deleteObjectsCalls = 0
	err = driver.DeleteObjects(ctx, []string{"file1.txt", "file2.txt"})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(deleteObjectsCalls),
		"two keys (under the 1000 batch size) must produce exactly 1 DeleteObjects call")
}

// TestS3DriverDeleteObjectsChunking1001Keys asserts that 1001 keys produce
// exactly 2 DeleteObjects calls, because the S3 driver chunks at 1000 keys.
func TestS3DriverDeleteObjectsChunking1001Keys(t *testing.T) {
	driver, deleteObjectsCalls := newDeleteObjectsTestDriver(t)
	ctx := context.Background()

	// Build 1001 distinct keys.
	keys := make([]string, 1001)
	for i := range keys {
		keys[i] = fmt.Sprintf("file%d.txt", i)
	}

	*deleteObjectsCalls = 0
	err := driver.DeleteObjects(ctx, keys)
	assert.NoError(t, err)

	// Chunk size is 1000, so 1001 keys must yield exactly 2 DeleteObjects calls
	// (1000 + 1).
	assert.Equal(t, int32(2), atomic.LoadInt32(deleteObjectsCalls),
		"1001 keys with batch size 1000 must produce exactly 2 DeleteObjects calls")
}

func TestS3DriverGetRange(t *testing.T) {
	body := []byte("0123456789")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bytes=2-5", r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes 2-5/10")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[2:6])
	}))
	defer server.Close()

	driver, err := storage.NewS3Driver(context.Background(), storage.S3Config{
		Bucket:          "b",
		Region:          "us-east-1",
		AccessKeyID:     "k",
		SecretAccessKey: "s",
		Endpoint:        server.URL,
		UsePathStyle:    true,
	})
	require.NoError(t, err)

	reader, total, err := driver.GetRange(context.Background(), "key.txt", 2, 4)
	require.NoError(t, err)
	defer reader.Close()
	got, _ := io.ReadAll(reader)
	assert.Equal(t, "2345", string(got))
	assert.Equal(t, int64(10), total)
}

func TestS3DriverGetRangeToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "bytes=5-", r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes 5-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("56789"))
	}))
	defer server.Close()

	driver, err := storage.NewS3Driver(context.Background(), storage.S3Config{
		Bucket: "b", Region: "us-east-1", AccessKeyID: "k", SecretAccessKey: "s",
		Endpoint: server.URL, UsePathStyle: true,
	})
	require.NoError(t, err)

	reader, total, err := driver.GetRange(context.Background(), "key.txt", 5, -1)
	require.NoError(t, err)
	defer reader.Close()
	got, _ := io.ReadAll(reader)
	assert.Equal(t, "56789", string(got))
	assert.Equal(t, int64(10), total)
}
