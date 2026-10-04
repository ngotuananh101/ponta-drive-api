package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ponta_drive/app/facades"
	_ "ponta_drive/tests"
)

// TestHTTPRequestTimeoutIsNotTheLethalDefault guards the root cause of the
// silent HTTP 408 on cloud connection tests.
//
// Goravel's gin router reads http.request_timeout and installs
// gin-contrib/timeout as the first global middleware. When that timeout fires
// the middleware writes a bare "408 Request Timeout" straight to the client and
// logs nothing, so the controller never runs its error path and the failure is
// undiagnosable. The framework default (and this project's previous hardcoded
// value) of 3 seconds is short enough that an external S3/MinIO round trip
// routinely trips it.
//
// A regression here is silent — it only shows up as an unexplained 408 in
// production — so the floor is asserted explicitly.
func TestHTTPRequestTimeoutIsNotTheLethalDefault(t *testing.T) {
	timeout := facades.Config().GetInt("http.request_timeout", 3)

	assert.Greater(t, timeout, 3,
		"http.request_timeout must be raised above the 3s framework default, "+
			"otherwise outbound cloud storage calls are aborted with a silent 408")
	assert.GreaterOrEqual(t, timeout, 15,
		"http.request_timeout must comfortably exceed the 15s bound the "+
			"cloud-account connection test applies to its own outbound calls, "+
			"so the controller can log the failure before the global timeout fires")
}
