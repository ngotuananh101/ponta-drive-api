package storage

import (
	"encoding/json"
	"errors"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
)

// CredentialService provides utilities to encrypt and decrypt cloud credentials.
type CredentialService struct{}

func NewCredentialService() *CredentialService {
	return &CredentialService{}
}

// EncryptCredentials serializes and encrypts S3Credentials using AES-256 via facades.Crypt().
func (s *CredentialService) EncryptCredentials(creds *models.S3Credentials) (string, error) {
	if creds == nil {
		return "", errors.New("credentials cannot be nil")
	}
	bytes, err := json.Marshal(creds)
	if err != nil {
		return "", err
	}
	return facades.Crypt().EncryptString(string(bytes))
}

// DecryptCredentials decrypts and deserializes the AES-256 ciphertext into S3Credentials.
func (s *CredentialService) DecryptCredentials(cipherText string) (*models.S3Credentials, error) {
	if cipherText == "" {
		return nil, errors.New("credentials cipher text is empty")
	}
	decrypted, err := facades.Crypt().DecryptString(cipherText)
	if err != nil {
		return nil, err
	}
	var creds models.S3Credentials
	if err := json.Unmarshal([]byte(decrypted), &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// MaskCredentials returns a sanitized map representation with SecretAccessKey omitted.
func (s *CredentialService) MaskCredentials(creds *models.S3Credentials) map[string]any {
	if creds == nil {
		return map[string]any{}
	}
	return map[string]any{
		"endpoint":       creds.Endpoint,
		"bucket":         creds.Bucket,
		"region":         creds.Region,
		"access_key_id":  creds.AccessKeyID,
		"use_path_style": creds.UsePathStyle,
		"public_url":     creds.PublicURL,
	}
}
