package requests

import (
	"github.com/goravel/framework/contracts/http"

	"ponta_drive/app/facades"
)

// attributes translates field names using validation.attributes.{field} from language files.
func attributes(ctx http.Context, fields ...string) map[string]string {
	result := make(map[string]string, len(fields))
	lang := facades.Lang(ctx)
	for _, field := range fields {
		key := "validation.attributes." + field
		translated := lang.Get(key)
		if translated != "" && translated != key {
			result[field] = translated
		} else {
			result[field] = field
		}
	}
	return result
}
