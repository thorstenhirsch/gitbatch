package command

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDynamicTimeout(t *testing.T) {
	base := 10 * time.Second

	tests := []struct {
		name     string
		changes  int
		expected time.Duration
	}{
		{"zero changes", 0, base},
		{"negative changes", -5, base},
		{"few changes", 50, base},
		{"exactly 100", 100, 2 * base},
		{"150 changes", 150, 2 * base},
		{"200 changes", 200, 3 * base},
		{"999 changes", 999, 10 * base},
		{"1000 changes", 1000, 11 * base},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DynamicTimeout(base, tt.changes)
			assert.Equal(t, tt.expected, result)
		})
	}
}
