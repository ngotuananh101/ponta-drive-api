package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type DriveSearchTestSuite struct {
	suite.Suite
	tests.TestCase
	user    models.User
	token   string
	account models.CloudAccount
}

func TestDriveSearchTestSuite(t *testing.T) {
	suite.Run(t, new(DriveSearchTestSuite))
}

func (s *DriveSearchTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Search User",
		Username: "search_" + uuid.New().String()[:8],
		Email:    "search_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

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

	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "Search Storage",
		Provider:  models.ProviderS3,
		IsDefault: true,
		IsActive:  true,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Bucket:          "search-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA...",
		SecretAccessKey: "secret...",
	})
	err = facades.Orm().Query().Create(&s.account)
	s.Require().NoError(err)
}

func (s *DriveSearchTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

func (s *DriveSearchTestSuite) createItem(name, itemType string, parentID *uint) models.DriveItem {
	item := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		ParentID:       parentID,
		Name:           name,
		Type:           itemType,
		Size:           10,
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&item))
	return item
}

// searchNames calls the list endpoint with `search` and no `parent_uuid` (the
// root context) and returns the names found.
func (s *DriveSearchTestSuite) searchNames(term string) []string {
	resp, err := s.Http(s.T()).WithToken(s.token).
		Get(fmt.Sprintf("/v1/drive/items?cloud_account_uuid=%s&search=%s", s.account.UUID, term))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)

	raw := body["data"].([]any)
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		names = append(names, entry.(map[string]any)["name"].(string))
	}
	return names
}

// Search must span the whole account, not just the folder in view: a file that
// lives inside a subfolder has to be found when searching from the root.
func (s *DriveSearchTestSuite) TestSearchSpansWholeAccount() {
	docs := s.createItem("Docs", models.ItemTypeFolder, nil)
	s.createItem("findme.txt", models.ItemTypeFile, &docs.ID)
	s.createItem("other.txt", models.ItemTypeFile, nil)

	names := s.searchNames("findme")

	s.Contains(names, "findme.txt", "a file inside a subfolder must be found from the root")
	s.NotContains(names, "other.txt")
}

// A search that matches nothing returns an empty list rather than the whole
// folder, and matching is a substring of the name.
func (s *DriveSearchTestSuite) TestSearchMatchesSubstringAndRejectsNonMatch() {
	s.createItem("quarterly-report.pdf", models.ItemTypeFile, nil)

	s.Contains(s.searchNames("report"), "quarterly-report.pdf")
	s.Empty(s.searchNames("zzz-no-such-name"), "a non-matching search must return nothing")
}
