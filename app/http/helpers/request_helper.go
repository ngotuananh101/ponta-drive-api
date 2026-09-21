package helpers

import (
	"net"
	"strings"

	"github.com/goravel/framework/contracts/http"
)

// GetClientIP extracts the real client IP from request headers (proxy-aware).
//
// Resolution order:
//  1. First valid entry of `X-Forwarded-For`
//  2. `X-Real-IP`
//  3. The remote address of the underlying connection
func GetClientIP(ctx http.Context) string {
	req := ctx.Request().Origin()

	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	return ip
}

// GetUserAgent extracts the User-Agent string from request headers.
func GetUserAgent(ctx http.Context) string {
	return ctx.Request().Header("User-Agent", "")
}
