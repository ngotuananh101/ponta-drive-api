package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type MultipartUploadTestSuite struct {
	suite.Suite
	tests.TestCase
	user       models.User
	token      string
	account    models.CloudAccount
	mockServer *httptest.Server
}

func TestMultipartUploadTestSuite(t *testing.T) {
	suite.Run(t, new(MultipartUploadTestSuite))
}

func (s *MultipartUploadTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	// Start mock S3 server supporting multipart upload requests
	s.mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		switch r.Method {
		case http.MethodPost:
			if _, ok := query["uploads"]; ok {
				// InitiateMultipart
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Bucket>mock-bucket</Bucket>
  <Key>` + r.URL.Path + `</Key>
  <UploadId>mock-upload-id-999</UploadId>
</InitiateMultipartUploadResult>`))
				return
			}
			if uploadID := query.Get("uploadId"); uploadID != "" {
				// CompleteMultipart
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Bucket>mock-bucket</Bucket>
  <Key>` + r.URL.Path + `</Key>
  <ETag>"mock-etag-final-multipart"</ETag>
</CompleteMultipartUploadResult>`))
				return
			}
		case http.MethodPut:
			if partNum := query.Get("partNumber"); partNum != "" {
				// UploadPart
				w.Header().Set("ETag", fmt.Sprintf(`"mock-part-%s-etag"`, partNum))
				w.WriteHeader(http.StatusOK)
				return
			}
		case http.MethodDelete:
			if uploadID := query.Get("uploadId"); uploadID != "" {
				// AbortMultipart
				w.WriteHeader(http.StatusNoContent)
				return
			}
		case http.MethodHead:
			w.Header().Set("Content-Length", "15728640")
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("ETag", `"mock-etag-final-multipart"`)
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Multipart User",
		Username: "mp_" + uuid.New().String()[:8],
		Email:    "mp_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	// Obtain JWT
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	dataMap := jsonBody["data"].(map[string]any)
	s.token = dataMap["token"].(string)

	// Create CloudAccount with mock endpoint
	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "Mock Multipart S3 Storage",
		Provider:  models.ProviderMinIO,
		IsDefault: true,
		IsActive:  true,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Endpoint:        s.mockServer.URL,
		Bucket:          "mock-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "mock-key",
		SecretAccessKey: "mock-secret",
		UsePathStyle:    true,
	})
	err = facades.Orm().Query().Create(&s.account)
	s.Require().NoError(err)
}

func (s *MultipartUploadTestSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.UploadSession{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *MultipartUploadTestSuite) TestMultipartUploadFullCycle() {
	// 1. Init Multipart Upload (POST /v1/drive/upload/multipart/init)
	initPayload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"file_name":        "archive.zip",
		"size":             15728640,
		"mime_type":        "application/zip",
		"chunk_size":       5242880,
	})

	initResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/upload/multipart/init", bytes.NewBuffer(initPayload))
	s.Require().NoError(err)
	initResp.AssertStatus(http.StatusOK)

	initBody, err := initResp.Json()
	s.Require().NoError(err)
	initData := initBody["data"].(map[string]any)
	sessionID := initData["session_id"].(string)
	s.NotEmpty(sessionID)
	s.Equal("mock-upload-id-999", initData["upload_id"])
	s.Equal(float64(3), initData["total_parts"])

	// 2. Upload 3 Parts (POST /v1/drive/upload/multipart/part)
	for partNum := 1; partNum <= 3; partNum++ {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		_ = writer.WriteField("session_id", sessionID)
		_ = writer.WriteField("part_number", fmt.Sprintf("%d", partNum))

		partData := bytes.Repeat([]byte("A"), 1024)
		partFile, err := writer.CreateFormFile("file", fmt.Sprintf("chunk_%d.bin", partNum))
		s.Require().NoError(err)
		_, _ = partFile.Write(partData)
		_ = writer.Close()

		partResp, err := s.Http(s.T()).WithToken(s.token).
			WithHeader("Content-Type", writer.FormDataContentType()).
			Post("/v1/drive/upload/multipart/part", &body)
		s.Require().NoError(err)
		partResp.AssertStatus(http.StatusOK)

		partBody, err := partResp.Json()
		s.Require().NoError(err)
		partResult := partBody["data"].(map[string]any)
		s.Equal(float64(partNum), partResult["part_number"])
		s.Equal(fmt.Sprintf("mock-part-%d-etag", partNum), partResult["etag"])
	}

	// 3. Complete Multipart Upload (POST /v1/drive/upload/multipart/complete)
	completePayload, _ := json.Marshal(map[string]any{
		"session_id": sessionID,
	})

	compResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/upload/multipart/complete", bytes.NewBuffer(completePayload))
	s.Require().NoError(err)
	compResp.AssertStatus(http.StatusOK)

	compBody, err := compResp.Json()
	s.Require().NoError(err)
	compData := compBody["data"].(map[string]any)
	s.Equal(models.ItemStatusReady, compData["status"])
	s.Equal("archive.zip", compData["name"])
	s.Equal("mock-etag-final-multipart", compData["etag"])

	// Verify DriveItem is persisted in database
	var item models.DriveItem
	err = facades.Orm().Query().Where("uuid", compData["uuid"]).First(&item)
	s.Require().NoError(err)
	s.Equal(models.ItemStatusReady, item.Status)
	s.Equal("zip", item.Extension)

	// Verify UploadSession was cleaned up
	var session models.UploadSession
	_ = facades.Orm().Query().Where("session_id", sessionID).First(&session)
	s.Zero(session.ID) // Record should be deleted
}

func (s *MultipartUploadTestSuite) TestMultipartUploadAbort() {
	// 1. Init Multipart Upload
	initPayload, _ := json.Marshal(map[string]any{
		"cloud_account_id": s.account.ID,
		"file_name":        "cancelled.zip",
		"size":             10485760,
	})

	initResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/upload/multipart/init", bytes.NewBuffer(initPayload))
	s.Require().NoError(err)
	initResp.AssertStatus(http.StatusOK)

	initBody, _ := initResp.Json()
	sessionID := initBody["data"].(map[string]any)["session_id"].(string)

	// 2. Abort Multipart Upload (POST /v1/drive/upload/multipart/abort)
	abortPayload, _ := json.Marshal(map[string]any{
		"session_id": sessionID,
	})
	abortResp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/drive/upload/multipart/abort", bytes.NewBuffer(abortPayload))
	s.Require().NoError(err)
	abortResp.AssertStatus(http.StatusOK)

	// Verify UploadSession was deleted
	var abortedSession models.UploadSession
	_ = facades.Orm().Query().Where("session_id", sessionID).First(&abortedSession)
	s.Zero(abortedSession.ID)
}
