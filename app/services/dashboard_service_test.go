package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0 B"},
		{"sub-kilobyte", 512, "512 B"},
		{"exactly one kilobyte", 1024, "1.0 KB"},
		{"fractional kilobyte", 1536, "1.5 KB"},
		{"one megabyte", 1048576, "1.0 MB"},
		{"one gigabyte", 1073741824, "1.0 GB"},
		{"one terabyte", 1099511627776, "1.0 TB"},
		{"just under a kilobyte", 1023, "1023 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, humanizeBytes(tt.input))
		})
	}
}
