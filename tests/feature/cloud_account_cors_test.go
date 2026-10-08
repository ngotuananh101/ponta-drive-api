package feature

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type CloudCorsTestSuite struct {
	suite.Suite
	tests.TestCase
	user  models.User
	token string
}

func TestCloudCorsTestSuite(t *testing.T) {
	suite.Run(t, new(CloudCorsTestSuite))
}

func (s *CloudCorsTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID: uuid.New().String(), Name: "CORS User",
		Username: "cors_" + uuid.New().String()[:8],
		Email:    "cors_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	s.Require().NoError(facades.Orm().Query().Create(&s.user))

	loginPayload, _ := json.Marshal(map[string]string{"email": s.user.Email, "password": "password123"})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()
	body, _ := resp.Json()
	s.token = body["data"].(map[string]any)["token"].(string)
}

func (s *CloudCorsTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

// TestCreateAccountSucceedsWhenCorsIsDenied pins the failure mode in the Review
// Focus: a key without s3:PutBucketCors must not stop the account being created.
func (s *CloudCorsTestSuite) TestCreateAccountSucceedsWhenCorsIsDenied() {
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code></Error>`))
	}))
	defer denied.Close()

	payload, _ := json.Marshal(map[string]any{
		"name": "Denied CORS", "provider": "minio",
		"endpoint": denied.URL, "bucket": "b", "region": "us-east-1",
		"access_key_id": "k", "secret_access_key": "s", "use_path_style": true,
	})
	resp, err := s.Http(s.T()).WithToken(s.token).Post("/v1/cloud-accounts", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusCreated)
}
