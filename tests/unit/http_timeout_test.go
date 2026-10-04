package unit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gintimeout "github.com/gin-contrib/timeout"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests reproduce, at the HTTP layer, the exact failure the cloud
// connection test hit: a request whose handler performs a slow outbound call
// is aborted by the global gin timeout middleware with a bare
// "408 Request Timeout" and nothing written to the application log.
//
// They are deliberately framework-level (plain gin + gin-contrib/timeout, the
// same middleware Goravel installs) so they need no database and run fast. The
// durations are scaled down from the real values (3s global / 15s bound) — the
// mechanism is identical, only the numbers differ.

// errSlowCall stands in for an S3/MinIO call that outlives its deadline.
var errSlowCall = errors.New("outbound call exceeded its deadline")

// slowCall blocks until ctx is done, then reports the deadline error — the
// behaviour a well-behaved SDK call has once it is handed a bounded context.
func slowCall(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return errSlowCall
	}
}

// TestGlobalTimeoutAbortsSlowHandlerWith408 pins the root cause: with a 3s
// global timeout (the value this project used to hardcode, and the Goravel
// default) a handler that runs longer is cut off with a bare 408 whose body is
// literally "Request Timeout". The middleware writes that response itself and
// logs nothing, so the failure is undiagnosable from the log file.
func TestGlobalTimeoutAbortsSlowHandlerWith408(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gintimeout.New(gintimeout.WithTimeout(300 * time.Millisecond)))
	router.GET("/slow", func(c *gin.Context) {
		// Unbounded outbound call, as the old controller code did with
		// context.Background().
		_ = slowCall(context.Background(), 2*time.Second)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))

	assert.Equal(t, http.StatusRequestTimeout, rec.Code)
	assert.Equal(t, "Request Timeout", rec.Body.String())
}

// TestBoundedOutboundCallBeatsGlobalTimeout pins the fix: when the handler
// bounds its outbound call with a deadline shorter than the global timeout, the
// call fails with an error, the handler runs its normal error path, and the
// client receives that response instead of a 408.
func TestBoundedOutboundCallBeatsGlobalTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Global timeout comfortably above the outbound bound, mirroring the
	// 60s global / 15s bound the real fix uses.
	router.Use(gintimeout.New(gintimeout.WithTimeout(2 * time.Second)))
	router.GET("/test", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Millisecond)
		defer cancel()

		if err := slowCall(ctx, 5*time.Second); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Connection test failed",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"status":"error","message":"Connection test failed"}`, rec.Body.String())
}
