package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"ponta_drive/app/services"
)

func TestPublicURLReachable(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	assert.True(t, services.PublicURLReachable(context.Background(), ok.URL+"/file.png"))

	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer missing.Close()
	assert.False(t, services.PublicURLReachable(context.Background(), missing.URL+"/file.png"))

	// An unreachable host must fail fast, not hang.
	assert.False(t, services.PublicURLReachable(context.Background(), "http://127.0.0.1:1/nope"))

	// A non-absolute URL is never treated as reachable.
	assert.False(t, services.PublicURLReachable(context.Background(), "/relative/path.png"))
	assert.False(t, services.PublicURLReachable(context.Background(), ""))
}