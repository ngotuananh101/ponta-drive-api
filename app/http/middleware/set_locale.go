package middleware

import (
	"strings"

	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
)

type SetLocale struct{}

func (m *SetLocale) Signature() string {
	return "set_locale"
}

func (m *SetLocale) Handle(ctx http.Context) {
	locale := ctx.Request().Query("lang", "")
	if locale == "" {
		locale = ctx.Request().Query("locale", "")
	}
	if locale == "" {
		locale = ctx.Request().Header("X-Locale", "")
	}
	if locale == "" {
		acceptLang := ctx.Request().Header("Accept-Language", "")
		if strings.HasPrefix(strings.ToLower(acceptLang), "en") {
			locale = "en"
		} else if strings.HasPrefix(strings.ToLower(acceptLang), "vi") {
			locale = "vi"
		}
	}

	if locale == "en" || locale == "vi" {
		facades.Lang(ctx).SetLocale(locale)
	}

	ctx.Request().Next()
}
