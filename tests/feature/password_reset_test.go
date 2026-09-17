package feature

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/goravel/framework/support/carbon"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

// hashToken returns the SHA-256 hash of the given token as a lowercase hex
// string. It mirrors the (unexported) hashing logic used by AuthController so
// that tests can insert and look up password reset tokens directly in the
// database while matching what the controller stores / compares against.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type PasswordResetTestSuite struct {
	suite.Suite
	tests.TestCase
	user models.User
}

func TestPasswordResetTestSuite(t *testing.T) {
	suite.Run(t, new(PasswordResetTestSuite))
}

func (s *PasswordResetTestSuite) SetupTest() {
	hashedPassword, _ := facades.Hash().Make("secret123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Test User",
		Username: "reset_" + uuid.New().String()[:8],
		Email:    "reset_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
		Avatar:   "https://ui-avatars.com/api/?name=Test+User&background=random",
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)
}

func (s *PasswordResetTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Delete(&s.user)
	}
	_, _ = facades.Orm().Query().Delete(&models.PasswordResetToken{})
}

// TestTableExists verifies the password_reset_tokens table is present.
func (s *PasswordResetTestSuite) TestTableExists() {
	s.True(facades.Schema().HasTable("password_reset_tokens"))
}

// TestCreateAndRetrieveToken verifies the PasswordResetToken model can be
// persisted and retrieved with its auto-managed CreatedAt timestamp.
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

// 1. TestForgotPasswordValidation
func (s *PasswordResetTestSuite) TestForgotPasswordValidation() {
	// 1.1 Empty payload (no email) -> 422 Unprocessable Entity
	emptyPayload, _ := json.Marshal(map[string]string{})
	resp, err := s.Http(s.T()).Post("/api/auth/forgot-password", bytes.NewBuffer(emptyPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)

	// 1.2 Invalid email format -> 422 Unprocessable Entity
	invalidEmailPayload, _ := json.Marshal(map[string]string{
		"email": "invalid-email",
	})
	resp, err = s.Http(s.T()).Post("/api/auth/forgot-password", bytes.NewBuffer(invalidEmailPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)
}

// 2. TestForgotPasswordWithNonExistentEmail
func (s *PasswordResetTestSuite) TestForgotPasswordWithNonExistentEmail() {
	payload, _ := json.Marshal(map[string]string{
		"email": "notfound@example.com",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/forgot-password", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("success", body["status"])
	s.Equal("Nếu email tồn tại trong hệ thống, liên kết đặt lại mật khẩu đã được gửi.", body["message"])

	// No token record should be created for a non-existent email.
	count, err := facades.Orm().Query().
		Model(&models.PasswordResetToken{}).
		Where("email = ?", "notfound@example.com").
		Count()
	s.Require().NoError(err)
	s.Equal(int64(0), count)
}

// 3. TestForgotPasswordSuccess
func (s *PasswordResetTestSuite) TestForgotPasswordSuccess() {
	payload, _ := json.Marshal(map[string]string{
		"email": s.user.Email,
	})
	resp, err := s.Http(s.T()).Post("/api/auth/forgot-password", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("success", body["status"])
	s.Equal("Nếu email tồn tại trong hệ thống, liên kết đặt lại mật khẩu đã được gửi.", body["message"])

	// A reset token record should now exist for the user's email.
	var record models.PasswordResetToken
	err = facades.Orm().Query().Where("email = ?", s.user.Email).First(&record)
	s.Require().NoError(err)
	s.NotEmpty(record.Token)
	s.Len(record.Token, 64) // SHA-256 hex digest (32 bytes -> 64 hex chars)
	s.NotNil(record.CreatedAt)
}

// 4. TestResetPasswordValidation
func (s *PasswordResetTestSuite) TestResetPasswordValidation() {
	// 4.1 Missing token -> 422
	missingTokenPayload, _ := json.Marshal(map[string]string{
		"email":                 s.user.Email,
		"password":              "newpassword123",
		"password_confirmation": "newpassword123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(missingTokenPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)

	// 4.2 Missing email -> 422
	missingEmailPayload, _ := json.Marshal(map[string]string{
		"token":                 "sometoken",
		"password":              "newpassword123",
		"password_confirmation": "newpassword123",
	})
	resp, err = s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(missingEmailPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)

	// 4.3 Password shorter than 6 characters -> 422
	shortPasswordPayload, _ := json.Marshal(map[string]string{
		"token":                 "sometoken",
		"email":                 s.user.Email,
		"password":              "12345",
		"password_confirmation": "12345",
	})
	resp, err = s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(shortPasswordPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)

	// 4.4 password_confirmation does not match password -> 422
	mismatchPayload, _ := json.Marshal(map[string]string{
		"token":                 "sometoken",
		"email":                 s.user.Email,
		"password":              "newpassword123",
		"password_confirmation": "differentpassword123",
	})
	resp, err = s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(mismatchPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)
}

// 5. TestResetPasswordInvalidToken
func (s *PasswordResetTestSuite) TestResetPasswordInvalidToken() {
	payload, _ := json.Marshal(map[string]string{
		"token":                 "invalidtoken",
		"email":                 s.user.Email,
		"password":              "newpassword123",
		"password_confirmation": "newpassword123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertStatus(400)

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("error", body["status"])
	s.Equal("Mã khôi phục không hợp lệ hoặc đã hết hạn.", body["message"])
}

// 6. TestResetPasswordExpiredToken
func (s *PasswordResetTestSuite) TestResetPasswordExpiredToken() {
	rawToken := "expired-raw-token"
	hashedToken := hashToken(rawToken)
	past := carbon.NewDateTime(carbon.Now().SubHours(2))

	tokenRecord := models.PasswordResetToken{
		Email:     s.user.Email,
		Token:     hashedToken,
		CreatedAt: past,
	}
	err := facades.Orm().Query().Create(&tokenRecord)
	s.Require().NoError(err)

	payload, _ := json.Marshal(map[string]string{
		"token":                 rawToken,
		"email":                 s.user.Email,
		"password":              "newpassword123",
		"password_confirmation": "newpassword123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertStatus(400)

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("error", body["status"])
	s.Equal("Mã khôi phục đã hết hạn. Vui lòng yêu cầu lại.", body["message"])

	// The expired token record should have been cleaned up by the controller.
	count, err := facades.Orm().Query().
		Model(&models.PasswordResetToken{}).
		Where("email = ?", s.user.Email).
		Count()
	s.Require().NoError(err)
	s.Equal(int64(0), count)
}

// 7. TestResetPasswordSuccess
func (s *PasswordResetTestSuite) TestResetPasswordSuccess() {
	rawToken := "valid-reset-token"
	hashedToken := hashToken(rawToken)
	now := carbon.NewDateTime(carbon.Now())

	tokenRecord := models.PasswordResetToken{
		Email:     s.user.Email,
		Token:     hashedToken,
		CreatedAt: now,
	}
	err := facades.Orm().Query().Create(&tokenRecord)
	s.Require().NoError(err)

	payload, _ := json.Marshal(map[string]string{
		"token":                 rawToken,
		"email":                 s.user.Email,
		"password":              "newpassword123",
		"password_confirmation": "newpassword123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("success", body["status"])
	s.Equal("Đặt lại mật khẩu thành công. Vui lòng đăng nhập bằng mật khẩu mới.", body["message"])

	// The one-time-use token should have been consumed (deleted).
	count, err := facades.Orm().Query().
		Model(&models.PasswordResetToken{}).
		Where("email = ?", s.user.Email).
		Count()
	s.Require().NoError(err)
	s.Equal(int64(0), count)

	// Login with the new password should succeed.
	newPwdPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "newpassword123",
	})
	newPwdResp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(newPwdPayload))
	s.Require().NoError(err)
	newPwdResp.AssertOk()
	newPwdBody, err := newPwdResp.Json()
	s.Require().NoError(err)
	s.Equal("success", newPwdBody["status"])

	// Login with the old password should fail.
	oldPwdPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "secret123",
	})
	oldPwdResp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(oldPwdPayload))
	s.Require().NoError(err)
	oldPwdResp.AssertUnauthorized()
}
