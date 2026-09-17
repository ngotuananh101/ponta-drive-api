package feature

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type PasswordResetTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestPasswordResetTestSuite(t *testing.T) {
	suite.Run(t, new(PasswordResetTestSuite))
}

func (s *PasswordResetTestSuite) SetupTest() {
}

func (s *PasswordResetTestSuite) TearDownTest() {
	facades.Orm().Query().Delete(&models.PasswordResetToken{})
}

func (s *PasswordResetTestSuite) TestTableExists() {
	s.True(facades.Schema().HasTable("password_reset_tokens"))
}

func (s *PasswordResetTestSuite) TestCreateAndRetrieveToken() {
	resetToken := models.PasswordResetToken{
		Email: "reset@example.com",
		Token: "random-reset-token-value",
	}

	err := facades.Orm().Query().Create(&resetToken)
	s.Require().NoError(err)
	s.NotZero(resetToken.CreatedAt)

	var found models.PasswordResetToken
	err = facades.Orm().Query().Where("token", "random-reset-token-value").First(&found)
	s.Require().NoError(err)
	s.Equal("reset@example.com", found.Email)
	s.Equal("random-reset-token-value", found.Token)
}
