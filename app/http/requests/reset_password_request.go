package requests

import (
	"strings"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/validation"
)

type ResetPasswordRequest struct {
	Token                string `form:"token" json:"token"`
	Email                string `form:"email" json:"email"`
	Password             string `form:"password" json:"password"`
	PasswordConfirmation string `form:"password_confirmation" json:"password_confirmation"`
}

func (r *ResetPasswordRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *ResetPasswordRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *ResetPasswordRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"token":                 "required",
		"email":                 "required|email",
		"password":              "required|min_len:6",
		"password_confirmation": "required|eq_field:password",
	}
}

func (r *ResetPasswordRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *ResetPasswordRequest) Attributes(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *ResetPasswordRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	if val, exist := data.Get("email"); exist && val != nil {
		if s, ok := val.(string); ok {
			_ = data.Set("email", strings.ToLower(strings.TrimSpace(s)))
		}
	}
	return nil
}
