package requests

import (
	"strings"

	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/validation"
)

type ForgotPasswordRequest struct {
	Email string `form:"email" json:"email"`
}

func (r *ForgotPasswordRequest) Authorize(ctx http.Context) error {
	return nil
}

func (r *ForgotPasswordRequest) Filters(ctx http.Context) map[string]any {
	return map[string]any{}
}

func (r *ForgotPasswordRequest) Rules(ctx http.Context) map[string]any {
	return map[string]any{
		"email": "required|email",
	}
}

func (r *ForgotPasswordRequest) Messages(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *ForgotPasswordRequest) Attributes(ctx http.Context) map[string]string {
	return map[string]string{}
}

func (r *ForgotPasswordRequest) PrepareForValidation(ctx http.Context, data validation.Data) error {
	if val, exist := data.Get("email"); exist && val != nil {
		if s, ok := val.(string); ok {
			_ = data.Set("email", strings.ToLower(strings.TrimSpace(s)))
		}
	}
	return nil
}
