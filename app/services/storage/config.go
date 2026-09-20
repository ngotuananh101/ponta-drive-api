package storage

import (
	"ponta_drive/app/models"
)

// S3Config holds configuration parameters for S3 and S3-compatible providers (AWS, R2, MinIO).
type S3Config struct {
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string
	UsePathStyle    bool
	PublicURL       string
}

// ConfigFromS3Credentials converts models.S3Credentials to S3Config.
func ConfigFromS3Credentials(creds *models.S3Credentials) S3Config {
	if creds == nil {
		return S3Config{}
	}
	return S3Config{
		Bucket:          creds.Bucket,
		Region:          creds.Region,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		Endpoint:        creds.Endpoint,
		UsePathStyle:    creds.UsePathStyle,
		PublicURL:       creds.PublicURL,
	}
}
