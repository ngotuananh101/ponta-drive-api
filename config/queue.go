package config

import (
	"ponta_drive/app/facades"
)

func init() {
	config := facades.Config()
	config.Add("queue", map[string]any{
		// Default Queue Connection Name.
		//
		// "sync" runs every dispatched job inline, inside the HTTP request that
		// dispatched it. "database" stores the job in the `jobs` table and a
		// worker running inside the server process picks it up, so the request
		// returns immediately. Set QUEUE_CONNECTION=database when a job (a full
		// bucket scan) can outlive the request timeout.
		"default": config.Env("QUEUE_CONNECTION", "sync"),

		// Queue Connections
		//
		// Here you may configure the connection information for each server that is used by your application.
		// Drivers: "sync", "database", "custom"
		"connections": map[string]any{
			"sync": map[string]any{
				"driver": "sync",
			},
			"database": map[string]any{
				"driver":     "database",
				"connection": config.Env("DB_CONNECTION"),
				"queue":      config.Env("QUEUE_NAME", "default"),
				"concurrent": config.Env("QUEUE_CONCURRENT", 1),
			},
		},

		// Failed Queue Jobs
		//
		// These options configure the behavior of failed queue job logging so you
		// can control how and where failed jobs are stored.
		"failed": map[string]any{
			"database": config.Env("DB_CONNECTION"),
			"table":    "failed_jobs",
		},
	})
}
