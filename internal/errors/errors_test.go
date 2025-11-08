package errors

import (
	"testing"
)

func TestParseGitErrorReturnsUnknownWhenEmpty(t *testing.T) {
	output := ParseGitError("", nil)
	if output == nil {
		t.Fatalf("expected error, got nil")
	}
	if output.Error() != "unknown error" {
		t.Fatalf("expected 'unknown error', got %q", output.Error())
	}
}

func TestParseGitErrorReturnsTrimmedOutput(t *testing.T) {
	output := ParseGitError("fatal: failure\n", nil)
	if output == nil {
		t.Fatalf("expected error, got nil")
	}
	if output.Error() != "fatal: failure" {
		t.Fatalf("expected trimmed message, got %q", output.Error())
	}
}
