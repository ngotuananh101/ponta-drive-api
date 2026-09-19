package requests

import (
	"strings"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/validation"
)

type LoginRequest struct {
	Email    string `form:"email" json:"email"`
	Password string `form:"password" json:"password"`
}

func (r *LoginRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *LoginRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *LoginRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"email":    "required|email",
		"password": "required",
	}
}

func (r *LoginRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *LoginRequest) Attributes(ctx http.Context) map[string]string {
	return attributes(ctx, "email", "password")
}

func (r *LoginRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	if val, exist := data.Get("email"); exist && val != nil {
		if s, ok := val.(string); ok {
			_ = data.Set("email", strings.ToLower(strings.TrimSpace(s)))
		}
	}
	return nil
}
