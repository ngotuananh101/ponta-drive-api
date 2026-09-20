package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"ponta_drive/app/contracts"
)

// S3Driver implements contracts.CloudDriver for AWS S3 and S3-compatible storage.
type S3Driver struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	cfg           S3Config
	bucket        string
	region        string
}

// NewS3Driver creates an initialized S3Driver instance.
func NewS3Driver(ctx context.Context, cfg S3Config) (*S3Driver, error) {
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	cfg.Region = region

	credProvider := credentials.NewStaticCredentialsProvider(
		cfg.AccessKeyID,
		cfg.SecretAccessKey,
		"",
	)

	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credProvider),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if strings.TrimSpace(cfg.Endpoint) != "" {
			o.BaseEndpoint = aws.String(strings.TrimSpace(cfg.Endpoint))
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	presignClient := s3.NewPresignClient(client)

	return &S3Driver{
		client:        client,
		presignClient: presignClient,
		cfg:           cfg,
		bucket:        cfg.Bucket,
		region:        region,
	}, nil
}

// GetBucket returns the configured bucket name.
func (d *S3Driver) GetBucket() string {
	return d.bucket
}

// GetRegion returns the configured region.
func (d *S3Driver) GetRegion() string {
	return d.region
}

// GetPublicURL returns the optional public CDN/custom domain URL.
func (d *S3Driver) GetPublicURL() string {
	return d.cfg.PublicURL
}

// GetClient returns the underlying s3.Client.
func (d *S3Driver) GetClient() *s3.Client {
	return d.client
}

// Put uploads an object stream directly to S3.
func (d *S3Driver) Put(ctx context.Context, key string, reader io.Reader, size int64, mimeType string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
		Body:   reader,
	}
	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}
	if mimeType != "" {
		input.ContentType = aws.String(mimeType)
	}

	_, err := d.client.PutObject(ctx, input)
	return err
}

// Get retrieves an object from S3 as an io.ReadCloser.
func (d *S3Driver) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := d.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}

// Delete deletes an object from S3.
func (d *S3Driver) Delete(ctx context.Context, key string) error {
	_, err := d.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	return err
}

// Exists checks if an object exists in S3 without retrieving its contents.
func (d *S3Driver) Exists(ctx context.Context, key string) (bool, error) {
	_, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return false, nil
		}
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			code := apiErr.ErrorCode()
			if code == "NotFound" || code == "NoSuchKey" || code == "404" {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

// GetMetadata retrieves metadata of an S3 object.
func (d *S3Driver) GetMetadata(ctx context.Context, key string) (*contracts.ObjectMetadata, error) {
	output, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	meta := &contracts.ObjectMetadata{
		Key: key,
	}
	if output.ContentLength != nil {
		meta.Size = *output.ContentLength
	}
	if output.ContentType != nil {
		meta.MimeType = *output.ContentType
	}
	if output.ETag != nil {
		meta.ETag = strings.Trim(*output.ETag, "\"")
	}
	if output.LastModified != nil {
		meta.LastModified = *output.LastModified
	}

	return meta, nil
}

// GetPresignedUploadURL generates a pre-signed PUT URL for direct client-to-S3 uploads.
func (d *S3Driver) GetPresignedUploadURL(ctx context.Context, key string, mimeType string, expire time.Duration) (*contracts.PresignedURLResponse, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}
	if mimeType != "" {
		input.ContentType = aws.String(mimeType)
	}

	presignedReq, err := d.presignClient.PresignPutObject(ctx, input, func(po *s3.PresignOptions) {
		po.Expires = expire
	})
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for k, v := range presignedReq.SignedHeader {
		if len(v) > 0 {
			headers[k] = strings.Join(v, ", ")
		}
	}

	return &contracts.PresignedURLResponse{
		URL:     presignedReq.URL,
		Method:  presignedReq.Method,
		Headers: headers,
		Expires: time.Now().Add(expire),
	}, nil
}

// GetPresignedDownloadURL generates a pre-signed GET URL for secure direct downloads.
func (d *S3Driver) GetPresignedDownloadURL(ctx context.Context, key string, fileName string, expire time.Duration) (string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}
	if strings.TrimSpace(fileName) != "" {
		input.ResponseContentDisposition = aws.String(fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	}

	presignedReq, err := d.presignClient.PresignGetObject(ctx, input, func(po *s3.PresignOptions) {
		po.Expires = expire
	})
	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}

// InitiateMultipart starts a multipart upload session and returns an upload ID.
func (d *S3Driver) InitiateMultipart(ctx context.Context, key string, mimeType string) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}
	if mimeType != "" {
		input.ContentType = aws.String(mimeType)
	}

	output, err := d.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", err
	}
	if output.UploadId == nil {
		return "", errors.New("empty upload id returned from S3")
	}

	return *output.UploadId, nil
}

// UploadPart uploads a single part of a multipart upload directly from reader without caching to disk.
func (d *S3Driver) UploadPart(ctx context.Context, key string, uploadID string, partNumber int32, reader io.Reader, size int64) (string, error) {
	input := &s3.UploadPartInput{
		Bucket:        aws.String(d.bucket),
		Key:           aws.String(key),
		UploadId:      aws.String(uploadID),
		PartNumber:    aws.Int32(partNumber),
		Body:          reader,
		ContentLength: aws.Int64(size),
	}

	output, err := d.client.UploadPart(ctx, input)
	if err != nil {
		return "", err
	}
	if output.ETag == nil {
		return "", errors.New("empty ETag returned from upload part")
	}

	return strings.Trim(*output.ETag, "\""), nil
}

// CompleteMultipart finalizes a multipart upload with the given completed parts.
func (d *S3Driver) CompleteMultipart(ctx context.Context, key string, uploadID string, parts []contracts.CompletedPart) error {
	completedParts := make([]types.CompletedPart, 0, len(parts))
	for _, p := range parts {
		etag := p.ETag
		if !strings.HasPrefix(etag, "\"") {
			etag = fmt.Sprintf("\"%s\"", etag)
		}
		completedParts = append(completedParts, types.CompletedPart{
			PartNumber: aws.Int32(p.PartNumber),
			ETag:       aws.String(etag),
		})
	}

	_, err := d.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(d.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	return err
}

// AbortMultipart cancels a multipart upload and frees S3 storage chunks.
func (d *S3Driver) AbortMultipart(ctx context.Context, key string, uploadID string) error {
	_, err := d.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(d.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	return err
}

// ListObjects lists objects under a given prefix, supporting continuation tokens and pagination.
func (d *S3Driver) ListObjects(ctx context.Context, prefix string, continuationToken string, maxKeys int32) (*contracts.ListObjectsResult, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(d.bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	if continuationToken != "" {
		input.ContinuationToken = aws.String(continuationToken)
	}
	if maxKeys > 0 {
		input.MaxKeys = aws.Int32(maxKeys)
	}

	output, err := d.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	result := &contracts.ListObjectsResult{
		IsTruncated: output.IsTruncated != nil && *output.IsTruncated,
	}
	if output.NextContinuationToken != nil {
		result.NextContinuation = *output.NextContinuationToken
	}

	for _, item := range output.Contents {
		meta := contracts.ObjectMetadata{}
		if item.Key != nil {
			meta.Key = *item.Key
		}
		if item.Size != nil {
			meta.Size = *item.Size
		}
		if item.ETag != nil {
			meta.ETag = strings.Trim(*item.ETag, "\"")
		}
		if item.LastModified != nil {
			meta.LastModified = *item.LastModified
		}
		result.Objects = append(result.Objects, meta)
	}

	for _, cp := range output.CommonPrefixes {
		if cp.Prefix != nil {
			result.CommonPrefixes = append(result.CommonPrefixes, *cp.Prefix)
		}
	}

	return result, nil
}
