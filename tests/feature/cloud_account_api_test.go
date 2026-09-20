package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type CloudAccountAPITestSuite struct {
	suite.Suite
	tests.TestCase
	user  models.User
	token string
}

func TestCloudAccountAPITestSuite(t *testing.T) {
	suite.Run(t, new(CloudAccountAPITestSuite))
}

func (s *CloudAccountAPITestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "API User",
		Username: "apiuser_" + uuid.New().String()[:8],
		Email:    "apiuser_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	// Obtain JWT token
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	dataMap := jsonBody["data"].(map[string]any)
	s.token = dataMap["token"].(string)
	s.NotEmpty(s.token)
}

func (s *CloudAccountAPITestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *CloudAccountAPITestSuite) TestUnauthorizedAccess() {
	resp, err := s.Http(s.T()).Get("/api/v1/cloud-accounts")
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusUnauthorized)
}

func (s *CloudAccountAPITestSuite) TestCloudAccountLifecycle() {
	// 1. Create Cloud Account (POST /api/v1/cloud-accounts)
	createPayload, _ := json.Marshal(map[string]any{
		"name":              "Primary AWS",
		"provider":          models.ProviderS3,
		"endpoint":          "https://s3.us-east-1.amazonaws.com",
		"bucket":            "ponta-prod-bucket",
		"region":            "us-east-1",
		"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"is_default":        true,
		"total_storage":     10737418240, // 10GB
	})

	resp, err := s.Http(s.T()).WithToken(s.token).Post("/api/v1/cloud-accounts", bytes.NewBuffer(createPayload))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusCreated)

	createdBody, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("ok", createdBody["status"])
	accountData := createdBody["data"].(map[string]any)
	accountID := uint(accountData["id"].(float64))
	s.Greater(accountID, uint(0))
	s.Equal("Primary AWS", accountData["name"])
	s.Equal(models.ProviderS3, accountData["provider"])
	s.True(accountData["is_default"].(bool))

	// Ensure secret_access_key is NOT leaked
	credsData := accountData["credentials"].(map[string]any)
	s.Equal("ponta-prod-bucket", credsData["bucket"])
	s.NotContains(credsData, "secret_access_key")

	// 2. List Cloud Accounts (GET /api/v1/cloud-accounts)
	listResp, err := s.Http(s.T()).WithToken(s.token).Get("/api/v1/cloud-accounts")
	s.Require().NoError(err)
	listResp.AssertOk()

	listBody, err := listResp.Json()
	s.Require().NoError(err)
	accountsList := listBody["data"].([]any)
	s.Len(accountsList, 1)

	// 3. Show Cloud Account (GET /api/v1/cloud-accounts/{id})
	showResp, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/api/v1/cloud-accounts/%d", accountID))
	s.Require().NoError(err)
	showResp.AssertOk()

	showBody, err := showResp.Json()
	s.Require().NoError(err)
	showData := showBody["data"].(map[string]any)
	s.Equal("Primary AWS", showData["name"])

	// 4. Update Cloud Account (PUT /api/v1/cloud-accounts/{id})
	updatePayload, _ := json.Marshal(map[string]any{
		"name": "Updated Primary AWS",
	})
	updateResp, err := s.Http(s.T()).WithToken(s.token).Put(fmt.Sprintf("/api/v1/cloud-accounts/%d", accountID), bytes.NewBuffer(updatePayload))
	s.Require().NoError(err)
	updateResp.AssertOk()

	updateBody, err := updateResp.Json()
	s.Require().NoError(err)
	updatedData := updateBody["data"].(map[string]any)
	s.Equal("Updated Primary AWS", updatedData["name"])

	// 5. Delete Cloud Account (DELETE /api/v1/cloud-accounts/{id})
	deleteResp, err := s.Http(s.T()).WithToken(s.token).Delete(fmt.Sprintf("/api/v1/cloud-accounts/%d", accountID), nil)
	s.Require().NoError(err)
	deleteResp.AssertOk()

	// Verify it is no longer returned in list
	listAfterResp, err := s.Http(s.T()).WithToken(s.token).Get("/api/v1/cloud-accounts")
	s.Require().NoError(err)
	listAfterBody, _ := listAfterResp.Json()
	s.Empty(listAfterBody["data"].([]any))
}

func (s *CloudAccountAPITestSuite) TestCloudAccountTestConnection() {
	// Mock S3 server for testing connection endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		xmlResp := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Name>test-bucket</Name>
    <Prefix></Prefix>
    <KeyCount>0</KeyCount>
    <MaxKeys>1</MaxKeys>
    <IsTruncated>false</IsTruncated>
</ListBucketResult>`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(xmlResp))
	}))
	defer mockServer.Close()

	testPayload, _ := json.Marshal(map[string]any{
		"provider":          models.ProviderMinIO,
		"endpoint":          mockServer.URL,
		"bucket":            "test-bucket",
		"region":            "us-east-1",
		"access_key_id":     "admin",
		"secret_access_key": "password",
		"use_path_style":    true,
	})

	resp, err := s.Http(s.T()).WithToken(s.token).Post("/api/v1/cloud-accounts/test", bytes.NewBuffer(testPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("ok", jsonBody["status"])
	s.Equal("Kết nối thành công", jsonBody["message"])

	// Test English locale via Accept-Language header
	respEn, err := s.Http(s.T()).WithToken(s.token).WithHeader("Accept-Language", "en").Post("/api/v1/cloud-accounts/test", bytes.NewBuffer(testPayload))
	s.Require().NoError(err)
	respEn.AssertOk()
	jsonBodyEn, err := respEn.Json()
	s.Require().NoError(err)
	s.Equal("ok", jsonBodyEn["status"])
	s.Equal("Connection successful", jsonBodyEn["message"])
}
