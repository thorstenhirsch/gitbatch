package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareNamesInsensitive(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// Identical strings
		{"abc", "abc", 0},
		{"ABC", "ABC", 0},
		{"", "", 0},

		// Case-insensitive ordering
		{"abc", "abd", -1},
		{"abd", "abc", 1},
		{"a", "B", -1},
		{"B", "a", 1},
		{"repo-alpha", "repo-beta", -1},
		{"repo-beta", "repo-alpha", 1},

		// Length differences
		{"abc", "abcd", -1},
		{"abcd", "abc", 1},
		{"a", "", 1},
		{"", "a", -1},

		// Case-sensitive tiebreaking: uppercase letters have lower
		// codepoints than lowercase, so "A" < "a" and "ABC" < "abc"
		{"ABC", "abc", -1},
		{"abc", "ABC", 1},
		{"Abc", "aBc", -1},
		{"Repo", "repo", -1},
		{"repo", "Repo", 1},
	}
	for _, tt := range tests {
		got := CompareNamesInsensitive(tt.a, tt.b)
		assert.Equal(t, tt.want, got, "CompareNamesInsensitive(%q, %q)", tt.a, tt.b)
	}
}
