package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/mail"
	"github.com/goravel/framework/support/carbon"

	"ponta_drive/app/facades"
	"ponta_drive/app/http/requests"
	"ponta_drive/app/models"
)

type AuthController struct{}

func NewAuthController() *AuthController {
	return &AuthController{}
}

// generateRandomToken creates a random 64-character hex token (32 bytes of entropy).
func generateRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// hashToken returns the SHA-256 hash of the given token as a hex string.
// Only the hash is persisted so that a database leak cannot be used to
// directly forge password reset links.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (c *AuthController) Login(ctx http.Context) http.Response {
	var loginRequest requests.LoginRequest
	errors, err := ctx.Request().ValidateRequest(&loginRequest)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}
	if errors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": errors.One(),
			"errors":  errors.All(),
		})
	}

	query, err := facades.SafeQuery()
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}

	var user models.User
	err = query.
		Where("email = ?", loginRequest.Email).
		First(&user)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}
	if user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.invalid_credentials"),
		})
	}

	if !facades.Hash().Check(loginRequest.Password, user.Password) {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.invalid_credentials"),
		})
	}

	token, err := facades.Auth(ctx).Login(&user)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.token_generate_failed"),
		})
	}

	ttl := facades.Config().GetInt("jwt.ttl", 60) * 60

	return ctx.Response().Success().Json(http.Json{
		"status": "success",
		"data": http.Json{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": ttl,
			"user":       user.ToResponse(),
		},
	})
}

func (c *AuthController) Me(ctx http.Context) http.Response {
	if val := ctx.Value("user"); val != nil {
		if user, ok := val.(models.User); ok {
			return ctx.Response().Success().Json(http.Json{
				"status": "success",
				"data":   user.ToResponse(),
			})
		}
	}

	var user models.User
	if err := facades.Auth(ctx).User(&user); err != nil || user.ID == 0 {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.unauthenticated"),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status": "success",
		"data":   user.ToResponse(),
	})
}

func (c *AuthController) Refresh(ctx http.Context) http.Response {
	token, err := facades.Auth(ctx).Refresh()
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.token_refresh_failed"),
		})
	}

	ttl := facades.Config().GetInt("jwt.ttl", 60) * 60

	return ctx.Response().Success().Json(http.Json{
		"status": "success",
		"data": http.Json{
			"token":      token,
			"token_type": "Bearer",
			"expires_in": ttl,
		},
	})
}

func (c *AuthController) Logout(ctx http.Context) http.Response {
	if err := facades.Auth(ctx).Logout(); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.logout_failed"),
		})
	}

	return ctx.Response().Success().Json(http.Json{
		"status":  "success",
		"message": facades.Lang(ctx).Get("auth.logout_success"),
	})
}

// ForgotPassword sends a password reset link to the user's email.
// To prevent user-enumeration, a successful request always returns the
// same success response regardless of whether the email exists.
func (c *AuthController) ForgotPassword(ctx http.Context) http.Response {
	var req requests.ForgotPasswordRequest
	validationErrors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}
	if validationErrors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": validationErrors.One(),
			"errors":  validationErrors.All(),
		})
	}

	// Success message returned to the client regardless of whether the user
	// exists, so the endpoint cannot be used to enumerate registered emails.
	successMessage := facades.Lang(ctx).Get("auth.forgot_password_success")

	query, err := facades.SafeQuery()
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}

	var user models.User
	err = query.
		Where("email = ?", req.Email).
		First(&user)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}
	if user.ID == 0 {
		return ctx.Response().Success().Json(http.Json{
			"status":  "success",
			"message": successMessage,
		})
	}

	rawToken, _ := generateRandomToken()
	hashedToken := hashToken(rawToken)

	// Remove any previously issued reset token for this email and store a new one.
	_, _ = query.Where("email = ?", req.Email).Delete(&models.PasswordResetToken{})

	resetRecord := models.PasswordResetToken{
		Email: req.Email,
		Token: hashedToken,
	}
	_ = query.Create(&resetRecord)

	frontendUrl := facades.Config().GetString("app.frontend_url", "http://localhost:5173")
	resetUrl := fmt.Sprintf("%s/reset-password?token=%s&email=%s", frontendUrl, rawToken, url.QueryEscape(req.Email))

	emailSubject := facades.Lang(ctx).Get("auth.mail_reset_subject")

	// Send the reset email asynchronously so the request is not blocked (and
	// potential 408 request timeouts) by a slow external SMTP relay.
	go func() {
		_ = facades.Mail().To([]string{user.Email}).
			Subject(emailSubject).
			Content(mail.Content{
				HtmlView: "reset_password.html",
				With: map[string]any{
					"Subject":       emailSubject,
					"Name":          user.Name,
					"Email":         user.Email,
					"ResetUrl":      resetUrl,
					"ExpireMinutes": 60,
					"Year":          time.Now().Year(),
				},
			}).
			Send()
	}()

	return ctx.Response().Success().Json(http.Json{
		"status":  "success",
		"message": successMessage,
	})
}

// ResetPassword validates the reset token, checks its expiry, and updates the
// user's password when everything is valid.
func (c *AuthController) ResetPassword(ctx http.Context) http.Response {
	var req requests.ResetPasswordRequest
	validationErrors, err := ctx.Request().ValidateRequest(&req)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": err.Error(),
		})
	}
	if validationErrors != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, http.Json{
			"status":  "error",
			"message": validationErrors.One(),
			"errors":  validationErrors.All(),
		})
	}

	query, err := facades.SafeQuery()
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}

	hashedToken := hashToken(req.Token)

	var resetRecord models.PasswordResetToken
	err = query.
		Where("email = ? AND token = ?", req.Email, hashedToken).
		First(&resetRecord)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}
	// Goravel's Query().First() swallows gorm.ErrRecordNotFound and returns a
	// nil error, so an empty record must be checked explicitly (mirrors the
	// `user.ID == 0` guard used in Login).
	if resetRecord.Token == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.reset_token_invalid"),
		})
	}

	// Expired token (older than 60 minutes): clean it up and reject the request.
	if resetRecord.CreatedAt != nil && resetRecord.CreatedAt.DiffInMinutes(carbon.Now()) > 60 {
		_, _ = query.Where("email = ?", req.Email).Delete(&models.PasswordResetToken{})
		return ctx.Response().Json(http.StatusBadRequest, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.reset_token_expired"),
		})
	}

	var user models.User
	err = query.
		Where("email = ?", req.Email).
		First(&user)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.db_error"),
		})
	}
	if user.ID == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{
			"status":  "error",
			"message": facades.Lang(ctx).Get("auth.user_not_found"),
		})
	}

	newHash, _ := facades.Hash().Make(req.Password)
	user.Password = newHash
	_ = query.Save(&user)

	// Single-use token: invalidate it once the password has been reset.
	_, _ = query.Where("email = ?", req.Email).Delete(&models.PasswordResetToken{})

	return ctx.Response().Success().Json(http.Json{
		"status":  "success",
		"message": facades.Lang(ctx).Get("auth.reset_password_success"),
	})
}
