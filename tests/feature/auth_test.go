package feature

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/tests"
)

type AuthTestSuite struct {
	suite.Suite
	tests.TestCase
	user models.User
}

func TestAuthTestSuite(t *testing.T) {
	suite.Run(t, new(AuthTestSuite))
}

func (s *AuthTestSuite) SetupTest() {
	hashedPassword, _ := facades.Hash().Make("secret123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Test User",
		Username: "testuser_" + uuid.New().String()[:8],
		Email:    "test_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
		Avatar:   "https://ui-avatars.com/api/?name=Test+User&background=random",
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)
}

func (s *AuthTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Delete(&s.user)
	}
}

func (s *AuthTestSuite) TestLoginAndMe() {
	// 1. Login with email
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "secret123",
	})

	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)

	dataMap, ok := jsonBody["data"].(map[string]any)
	s.Require().True(ok)
	token, ok := dataMap["token"].(string)
	s.Require().True(ok)
	s.NotEmpty(token)

	// 2. Call /api/auth/me with Bearer token
	meResp, err := s.Http(s.T()).WithToken(token).Get("/api/auth/me")
	s.Require().NoError(err)
	meResp.AssertOk()

	meJson, err := meResp.Json()
	s.Require().NoError(err)
	userData := meJson["data"].(map[string]any)
	s.Equal(s.user.Username, userData["username"])
	s.Equal(s.user.Email, userData["email"])
	s.Equal(s.user.UUID, userData["uuid"])
}

func (s *AuthTestSuite) TestLoginValidation() {
	// 1. Empty payload -> 422
	emptyPayload, _ := json.Marshal(map[string]string{})
	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(emptyPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("error", jsonBody["status"])
	s.NotEmpty(jsonBody["message"])

	// 2. Missing password -> 422
	missingPwdPayload, _ := json.Marshal(map[string]string{
		"email": s.user.Email,
	})
	resp, err = s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(missingPwdPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)

	// 3. Invalid email format -> 422
	invalidEmailPayload, _ := json.Marshal(map[string]string{
		"email":    "not-an-email",
		"password": "secret123",
	})
	resp, err = s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(invalidEmailPayload))
	s.Require().NoError(err)
	resp.AssertStatus(422)
	invalidJson, err := resp.Json()
	s.Require().NoError(err)
	s.Contains(invalidJson["message"].(string), "địa chỉ email hợp lệ")

	// 4. Valid email and password -> 200
	validPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "secret123",
	})
	resp, err = s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(validPayload))
	s.Require().NoError(err)
	resp.AssertOk()
}

func (s *AuthTestSuite) TestLoginInvalidPassword() {
	payload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "wrongpassword",
	})

	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	resp.AssertUnauthorized()
}

func (s *AuthTestSuite) TestUnauthenticatedMe() {
	resp, err := s.Http(s.T()).Get("/api/auth/me")
	s.Require().NoError(err)
	resp.AssertUnauthorized()
}

func (s *AuthTestSuite) TestRefreshAndLogout() {
	// 1. Login
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "secret123",
	})
	resp, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	dataMap := jsonBody["data"].(map[string]any)
	token := dataMap["token"].(string)

	// 2. Refresh
	refreshResp, err := s.Http(s.T()).WithToken(token).Post("/api/auth/refresh", nil)
	s.Require().NoError(err)
	refreshResp.AssertOk()

	refreshJson, err := refreshResp.Json()
	s.Require().NoError(err)
	refreshData := refreshJson["data"].(map[string]any)
	newToken := refreshData["token"].(string)
	s.NotEmpty(newToken)

	// 3. Logout
	logoutResp, err := s.Http(s.T()).WithToken(newToken).Post("/api/auth/logout", nil)
	s.Require().NoError(err)
	logoutResp.AssertOk()
}

func (s *AuthTestSuite) TestI18nLanguageSupport() {
	payload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "wrongpassword",
	})

	// 1. Default locale (vi)
	respVi, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	respVi.AssertUnauthorized()
	bodyVi, err := respVi.Json()
	s.Require().NoError(err)
	s.Equal("Email hoặc mật khẩu không chính xác", bodyVi["message"])

	// 2. English via Accept-Language header
	respEnHeader, err := s.Http(s.T()).WithHeader("Accept-Language", "en-US,en;q=0.9").Post("/api/auth/login", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	respEnHeader.AssertUnauthorized()
	bodyEnHeader, err := respEnHeader.Json()
	s.Require().NoError(err)
	s.Equal("Invalid email or password", bodyEnHeader["message"])

	// 3. English via query param ?lang=en
	respEnQuery, err := s.Http(s.T()).Post("/api/auth/login?lang=en", bytes.NewBuffer(payload))
	s.Require().NoError(err)
	respEnQuery.AssertUnauthorized()
	bodyEnQuery, err := respEnQuery.Json()
	s.Require().NoError(err)
	s.Equal("Invalid email or password", bodyEnQuery["message"])
}

func (s *AuthTestSuite) TestAttributeLocalization() {
	// 1. Vietnamese validation message for missing password
	viMissingPwd, _ := json.Marshal(map[string]string{
		"email": s.user.Email,
	})
	respVi, err := s.Http(s.T()).Post("/api/auth/login", bytes.NewBuffer(viMissingPwd))
	s.Require().NoError(err)
	respVi.AssertStatus(422)
	bodyVi, err := respVi.Json()
	s.Require().NoError(err)
	s.Equal("Trường mật khẩu là bắt buộc.", bodyVi["message"])

	// 2. English validation message for missing password
	respEn, err := s.Http(s.T()).WithHeader("Accept-Language", "en").Post("/api/auth/login", bytes.NewBuffer(viMissingPwd))
	s.Require().NoError(err)
	respEn.AssertStatus(422)
	bodyEn, err := respEn.Json()
	s.Require().NoError(err)
	s.Equal("The password field is required.", bodyEn["message"])

	// 3. Reset password mismatch attributes in Vietnamese
	viMismatch, _ := json.Marshal(map[string]string{
		"token":                 "dummy-token",
		"email":                 s.user.Email,
		"password":              "newpassword123",
		"password_confirmation": "differentpassword",
	})
	respMismatchVi, err := s.Http(s.T()).Post("/api/auth/reset-password", bytes.NewBuffer(viMismatch))
	s.Require().NoError(err)
	respMismatchVi.AssertStatus(422)
	bodyMismatchVi, err := respMismatchVi.Json()
	s.Require().NoError(err)
	s.Equal("Trường xác nhận mật khẩu phải khớp với mật khẩu.", bodyMismatchVi["message"])

	// 4. Reset password mismatch attributes in English
	respMismatchEn, err := s.Http(s.T()).Post("/api/auth/reset-password?lang=en", bytes.NewBuffer(viMismatch))
	s.Require().NoError(err)
	respMismatchEn.AssertStatus(422)
	bodyMismatchEn, err := respMismatchEn.Json()
	s.Require().NoError(err)
	s.Equal("The password confirmation field must match password.", bodyMismatchEn["message"])
}

