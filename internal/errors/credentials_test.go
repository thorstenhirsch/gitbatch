package errors

import (
	"fmt"
	"testing"
)

func TestRequiresCredentials(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "ErrAuthenticationRequired",
			err:      ErrAuthenticationRequired,
			expected: true,
		},
		{
			name:     "ErrPermissionDenied",
			err:      ErrPermissionDenied,
			expected: true,
		},
		{
			name:     "ErrAuthorizationFailed",
			err:      ErrAuthorizationFailed,
			expected: true,
		},
		{
			name:     "authentication required message",
			err:      fmt.Errorf("authentication required"),
			expected: true,
		},
		{
			name:     "permission denied message",
			err:      fmt.Errorf("permission denied"),
			expected: true,
		},
		{
			name:     "HTTP Basic access denied",
			err:      fmt.Errorf("HTTP Basic: Access Denied"),
			expected: true,
		},
		{
			name:     "401 unauthorized",
			err:      fmt.Errorf("401 unauthorized"),
			expected: true,
		},
		{
			name:     "403 forbidden",
			err:      fmt.Errorf("403 forbidden"),
			expected: true,
		},
		{
			name:     "fatal authentication error",
			err:      fmt.Errorf("fatal: Authentication failed"),
			expected: true,
		},
		{
			name:     "could not read username",
			err:      fmt.Errorf("could not read username"),
			expected: true,
		},
		{
			name:     "could not read password",
			err:      fmt.Errorf("could not read password"),
			expected: true,
		},
		{
			name:     "invalid username or password",
			err:      fmt.Errorf("invalid username or password"),
			expected: true,
		},
		{
			name:     "non-auth error",
			err:      ErrRemoteBranchNotSpecified,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "unrelated error",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RequiresCredentials(tt.err)
			if result != tt.expected {
				t.Errorf("RequiresCredentials(%v) = %v; want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestParseGitErrorDetectsAuthentication(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected error
	}{
		{
			name:     "authentication required",
			output:   "fatal: Authentication required for https://github.com/user/repo.git/",
			expected: ErrAuthenticationRequired,
		},
		{
			name:     "authentication failed",
			output:   "fatal: Authentication failed",
			expected: ErrAuthenticationRequired,
		},
		{
			name:     "HTTP basic access denied",
			output:   "fatal: HTTP Basic: Access denied",
			expected: ErrAuthenticationRequired,
		},
		{
			name:     "could not read username",
			output:   "could not read username for 'https://github.com'",
			expected: ErrAuthenticationRequired,
		},
		{
			name:     "permission denied publickey",
			output:   "Permission denied (publickey)",
			expected: ErrPermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseGitError(tt.output, nil)
			if result != tt.expected {
				t.Errorf("ParseGitError(%q) = %v; want %v", tt.output, result, tt.expected)
			}
		})
	}
}
