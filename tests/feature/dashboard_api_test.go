package feature

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type DashboardAPITestSuite struct {
	suite.Suite
	tests.TestCase
	user  models.User
	token string
}

func TestDashboardAPITestSuite(t *testing.T) {
	suite.Run(t, new(DashboardAPITestSuite))
}

func (s *DashboardAPITestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Dashboard API User",
		Username: "dashapi_" + uuid.New().String()[:8],
		Email:    "dashapi_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	s.Require().NoError(facades.Orm().Query().Create(&s.user))

	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	s.token = jsonBody["data"].(map[string]any)["token"].(string)
	s.Require().NotEmpty(s.token)
}

func (s *DashboardAPITestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).Delete(&models.ActivityLog{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *DashboardAPITestSuite) TestUnauthorized() {
	resp, err := s.Http(s.T()).Get("/v1/dashboard/summary")
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusUnauthorized)
}

func (s *DashboardAPITestSuite) TestSummaryReturnsExpectedEnvelope() {
	cloud := models.CloudAccount{
		UserID:       s.user.ID,
		Name:         "Primary S3",
		Provider:     models.ProviderS3,
		IsDefault:    true,
		IsActive:     true,
		SyncStatus:   "idle",
		TotalStorage: 10 * 1024 * 1024 * 1024,
		UsedStorage:  2 * 1024 * 1024 * 1024,
	}
	s.Require().NoError(cloud.SetCredentials(&models.S3Credentials{
		Endpoint: "https://s3.us-east-1.amazonaws.com",
		Bucket:   "dash-bucket",
		Region:   "us-east-1",
	}))
	s.Require().NoError(facades.Orm().Query().Create(&cloud))

	resp, err := s.Http(s.T()).WithToken(s.token).Get("/v1/dashboard/summary")
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("ok", body["status"])

	data, ok := body["data"].(map[string]any)
	s.Require().True(ok, "response must carry a data object")

	// Every aggregate the home dashboard depends on must be present.
	s.Contains(data, "clouds")
	s.Contains(data, "suggested_files")
	s.Contains(data, "recent_activities")
	s.Contains(data, "total_storage")

	clouds := data["clouds"].([]any)
	s.Require().Len(clouds, 1)
	first := clouds[0].(map[string]any)
	s.Equal("Primary S3", first["name"])
	s.Equal("idle", first["sync_status"])

	storage := data["total_storage"].(map[string]any)
	s.Equal(float64(10*1024*1024*1024), storage["total_bytes"])
	s.Equal("10.0 GB", storage["total_human"])
}
