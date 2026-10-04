package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ponta_drive/app/facades"
	_ "ponta_drive/tests"
)

// The queue connection is now driven by QUEUE_CONNECTION instead of a
// hardcoded value, so an operator can move a slow job (a full bucket scan) off
// the request path by switching to the "database" driver.
//
// These tests pin the contract the ENV switch relies on: a default connection
// is always configured, it names a real connection, and both the inline
// ("sync") and worker-backed ("database") connections exist. A regression here
// silently removes the operator's escape hatch.

func TestQueueHasADefaultConnection(t *testing.T) {
	def := facades.Config().GetString("queue.default")

	assert.NotEmpty(t, def,
		"queue.default must resolve to a connection name; "+
			"an empty value makes every Dispatch() fail")
}

func TestQueueDefaultNamesAConfiguredConnection(t *testing.T) {
	def := facades.Config().GetString("queue.default")

	driver := facades.Config().GetString("queue.connections." + def + ".driver")

	assert.NotEmpty(t, driver,
		"queue.default points at %q but queue.connections.%s has no driver; "+
			"dispatched jobs would have nowhere to run", def, def)
}

func TestQueueProvidesSyncAndDatabaseConnections(t *testing.T) {
	assert.Equal(t, "sync", facades.Config().GetString("queue.connections.sync.driver"),
		`the "sync" connection must stay available: it is the default and runs `+
			`jobs inline`)

	assert.Equal(t, "database", facades.Config().GetString("queue.connections.database.driver"),
		`the "database" connection must stay available: it is the QUEUE_CONNECTION=database `+
			`escape hatch that keeps a slow bucket scan off the request path`)
}

func TestQueueDatabaseConnectionIsFullyConfigured(t *testing.T) {
	assert.NotEmpty(t, facades.Config().GetString("queue.connections.database.connection"),
		`the database queue must name a DB connection, otherwise the worker `+
			`cannot read the jobs table`)

	assert.NotEmpty(t, facades.Config().GetString("queue.connections.database.queue"),
		`the database queue must name a queue, otherwise jobs are stored under `+
			`an empty queue name and never picked up`)

	assert.GreaterOrEqual(t, facades.Config().GetInt("queue.connections.database.concurrent", 0), 1,
		`the database queue must allow at least one concurrent worker, otherwise `+
			`no job is ever processed`)
}
