package helpers

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	gincontext "github.com/gin-gonic/gin"
	ginctx "github.com/goravel/gin"
	"github.com/stretchr/testify/assert"
)

// newRequestContext builds a real Goravel http.Context backed by a gin context
// so the helpers read from an actual *http.Request rather than a nil stub.
func newRequestContext(t *testing.T, mutate func(*nethttp.Request)) *ginctx.Context {
	t.Helper()

	w := httptest.NewRecorder()
	ginCtx, _ := gincontext.CreateTestContext(w)
	ginCtx.Request = httptest.NewRequest(nethttp.MethodGet, "/", nil)
	if mutate != nil {
		mutate(ginCtx.Request)
	}

	return ginctx.NewContext(ginCtx)
}

func TestGetClientIPPrefersXForwardedFor(t *testing.T) {
	ctx := newRequestContext(t, func(r *nethttp.Request) {
		r.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")
	})

	assert.Equal(t, "203.0.113.1", GetClientIP(ctx))
}

func TestGetClientIPFallsBackToXRealIP(t *testing.T) {
	ctx := newRequestContext(t, func(r *nethttp.Request) {
		r.Header.Set("X-Real-IP", "203.0.113.5")
	})

	assert.Equal(t, "203.0.113.5", GetClientIP(ctx))
}

func TestGetClientIPSkipsInvalidXForwardedFor(t *testing.T) {
	// A malformed XFF entry must not be returned verbatim; the helper falls
	// through to X-Real-IP.
	ctx := newRequestContext(t, func(r *nethttp.Request) {
		r.Header.Set("X-Forwarded-For", "not-an-ip, 198.51.100.1")
		r.Header.Set("X-Real-IP", "203.0.113.5")
	})

	assert.Equal(t, "203.0.113.5", GetClientIP(ctx))
}

func TestGetClientIPFallsBackToRemoteAddr(t *testing.T) {
	ctx := newRequestContext(t, func(r *nethttp.Request) {
		r.RemoteAddr = "192.168.1.100:12345"
	})

	assert.Equal(t, "192.168.1.100", GetClientIP(ctx))
}

func TestGetUserAgent(t *testing.T) {
	ctx := newRequestContext(t, func(r *nethttp.Request) {
		r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	})

	assert.Equal(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", GetUserAgent(ctx))
}
