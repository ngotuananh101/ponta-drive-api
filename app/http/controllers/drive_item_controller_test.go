package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// This file deliberately imports nothing from ponta_drive/tests: doing so would
// pull bootstrap -> routes -> controllers and create an import cycle.

func TestShouldLogRename(t *testing.T) {
	tests := []struct {
		name         string
		hasPrevious  bool
		newName      string
		previousName string
		expected     bool
	}{
		{
			name:         "renames are logged",
			hasPrevious:  true,
			newName:      "holiday-2026.jpg",
			previousName: "holiday.jpg",
			expected:     true,
		},
		{
			name:         "unchanged name is not logged",
			hasPrevious:  true,
			newName:      "holiday.jpg",
			previousName: "holiday.jpg",
			expected:     false,
		},
		{
			name:         "empty new name is not logged",
			hasPrevious:  true,
			newName:      "",
			previousName: "holiday.jpg",
			expected:     false,
		},
		{
			// The bug this guards: a failed lookup used to default previousName to
			// "" and then log a rename that never happened.
			name:         "failed lookup is not logged",
			hasPrevious:  false,
			newName:      "holiday.jpg",
			previousName: "",
			expected:     false,
		},
		{
			name:         "naming a previously unnamed item is logged",
			hasPrevious:  true,
			newName:      "Untitled",
			previousName: "",
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected,
				shouldLogRename(tt.hasPrevious, tt.newName, tt.previousName))
		})
	}
}
