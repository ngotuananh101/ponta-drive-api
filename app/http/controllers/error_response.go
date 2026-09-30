package controllers

import (
	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
)

// failResponse builds the JSON body returned to a client when an operation
// fails.
//
// The underlying error is deliberately NOT included in the response: ORM and
// driver errors carry schema names, table names, constraint names and query
// fragments, none of which a client should see. The real error is written to
// the application log instead, so the failure stays diagnosable.
//
// A missing translation key falls back to the key itself (goravel behaviour),
// so the client never receives an empty message.
func failResponse(ctx http.Context, status int, langKey string, err error) http.Response {
	if err != nil {
		facades.Log().Errorf("[API] %s: %v", langKey, err)
	}

	return ctx.Response().Json(status, http.Json{
		"status":  "error",
		"message": facades.Lang(ctx).Get(langKey),
	})
}
