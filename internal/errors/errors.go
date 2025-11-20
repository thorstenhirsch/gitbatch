package errors

import (
	"fmt"
	"strings"
)

// GitError is the errors from git package
type GitError string

const (
	// ErrGitCommand is thrown when git command returned an error code
	ErrGitCommand GitError = "git command returned error code"
	// ErrAuthenticationRequired is thrown when an authentication required on
	// a remote operation
	ErrAuthenticationRequired GitError = "authentication required"
	// ErrAuthorizationFailed is thrown when authorization failed while trying
	// to authenticate with remote
	ErrAuthorizationFailed GitError = "authorization failed"
	// ErrInvalidAuthMethod is thrown when invalid auth method is invoked
	ErrInvalidAuthMethod GitError = "invalid auth method"
	// ErrAlreadyUpToDate is thrown when a repository is already up to date
	// with its src on merge/fetch/pull
	ErrAlreadyUpToDate GitError = "already up to date"
	// ErrCouldNotFindRemoteRef is thrown when trying to fetch/pull cannot
	// find suitable remote reference
	ErrCouldNotFindRemoteRef GitError = "could not find remote ref"
	// ErrMergeAbortedTryCommit indicates that the repositort is not clean and
	// some changes may conflict with the merge
	ErrMergeAbortedTryCommit GitError = "stash/commit changes. aborted"
	// ErrRemoteBranchNotSpecified means that default remote branch is not set
	// for the current branch. can be setted with "git config --local --add
	// branch.<your branch name>.remote=<your remote name> "
	ErrRemoteBranchNotSpecified GitError = ("upstream not set")
	// ErrRemoteNotFound is thrown when the remote is not reachable. It may be
	// caused by the deletion of the remote or connectivity problems
	ErrRemoteNotFound GitError = ("remote not found")
	// ErrConflictAfterMerge is thrown when a conflict occurs at merging two
	// references
	ErrConflictAfterMerge GitError = ("conflict while merging")
	// ErrUnmergedFiles possibly occurs after a conflict
	ErrUnmergedFiles GitError = ("unmerged files detected")
	// ErrReferenceBroken thrown when unable to resolve reference
	ErrReferenceBroken GitError = ("unable to resolve reference")
	// ErrPermissionDenied is thrown when ssh authentication occurs
	ErrPermissionDenied GitError = ("permission denied")
	// ErrOverwrittenByMerge is the thrown when there is un-tracked files on working tree
	ErrOverwrittenByMerge GitError = ("move or remove un-tracked files before merge")
	// ErrUserEmailNotSet is thrown if there is no configured user email while
	// commit command
	ErrUserEmailNotSet GitError = "user email not set"
	// ErrCredentialPromptDetected is thrown when a credential prompt is detected in the output
	ErrCredentialPromptDetected GitError = "credential prompt detected"
	// ErrNetworkTimeout is thrown when network operations timeout
	ErrNetworkTimeout GitError = ("network timeout")
	// ErrNetworkUnreachable is thrown when network is unreachable
	ErrNetworkUnreachable GitError = ("network unreachable")
	// ErrDNSError is thrown when DNS resolution fails
	ErrDNSError GitError = ("dns resolution failed")
	// ErrSSLError is thrown when SSL/TLS validation fails
	ErrSSLError GitError = ("ssl certificate problem")
	// ErrUnclassified is unconsidered error type
	ErrUnclassified GitError = ("unclassified error")
	// NoErrIterationHalted is thrown for catching stops in interators
	NoErrIterationHalted GitError = ("iteration halted")
)

// gitErrorWithExitCode wraps a GitError with exit code information
type gitErrorWithExitCode struct {
	GitError
	exitCode int
}

func (e gitErrorWithExitCode) ExitCode() int {
	return e.exitCode
}

func (e gitErrorWithExitCode) Error() string {
	return e.GitError.Error()
}

func (e GitError) Error() string {
	return string(e)
}

type exitCoder interface {
	ExitCode() int
}

func exitCodeFromError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}

	if coder, ok := err.(exitCoder); ok {
		return coder.ExitCode(), true
	}
	return 0, false
}

// RequiresCredentials checks if the error indicates that authentication credentials are required
func RequiresCredentials(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific error types
	switch err {
	case ErrAuthenticationRequired, ErrPermissionDenied, ErrAuthorizationFailed, ErrCredentialPromptDetected:
		return true
	}

	// Check error message content
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "authentication required") ||
		strings.Contains(errMsg, "authentication failed") ||
		strings.Contains(errMsg, "could not read username") ||
		strings.Contains(errMsg, "could not read password") ||
		strings.Contains(errMsg, "invalid username or password") ||
		strings.Contains(errMsg, "http basic: access denied") ||
		strings.Contains(errMsg, "remote: http basic: access denied") ||
		strings.Contains(errMsg, "remote: invalid username or password") ||
		strings.Contains(errMsg, "fatal: authentication failed for") ||
		strings.Contains(errMsg, "permission denied (publickey)") ||
		strings.Contains(errMsg, "permission denied (password)") ||
		strings.Contains(errMsg, "fatal: authentication") ||
		strings.Contains(errMsg, "permission denied") ||
		strings.Contains(errMsg, "401 unauthorized") ||
		strings.Contains(errMsg, "403 forbidden")
}

// IsRecoverable checks if the error is recoverable (can be fixed by user action)
func IsRecoverable(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific error types that are recoverable
	switch err {
	case ErrAlreadyUpToDate, ErrRemoteBranchNotSpecified, ErrMergeAbortedTryCommit,
		ErrConflictAfterMerge, ErrUnmergedFiles, ErrOverwrittenByMerge,
		ErrRemoteNotFound, ErrNetworkTimeout, ErrNetworkUnreachable, ErrDNSError, ErrSSLError:
		return true
	}

	return false
}

// ParseGitError takes git output as an input and tries to find some meaningful
// errors can be used by the app
func ParseGitError(out string, err error) error {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" && err != nil {
		trimmed = strings.TrimSpace(err.Error())
	}

	// Get exit code if available
	var exitCode int
	if coder, ok := err.(exitCoder); ok {
		exitCode = coder.ExitCode()
	}

	// Check for authentication errors first
	lowerOut := strings.ToLower(out)
	lowerTrimmed := strings.ToLower(trimmed)
	if strings.Contains(lowerOut, "authentication required") ||
		strings.Contains(lowerOut, "authentication failed") ||
		strings.Contains(lowerOut, "could not read username") ||
		strings.Contains(lowerOut, "could not read password") ||
		strings.Contains(lowerOut, "invalid username or password") ||
		strings.Contains(lowerOut, "http basic: access denied") ||
		strings.Contains(lowerOut, "remote: http basic: access denied") ||
		strings.Contains(lowerOut, "remote: invalid username or password") ||
		strings.Contains(lowerOut, "fatal: authentication failed for") ||
		strings.Contains(lowerTrimmed, "fatal: authentication") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrAuthenticationRequired, exitCode: exitCode}
		}
		return ErrAuthenticationRequired
	}

	if strings.Contains(out, "error: Your local changes to the following files would be overwritten by merge") {
		return ErrMergeAbortedTryCommit
	} else if strings.Contains(out, "ERROR: Repository not found") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrRemoteNotFound, exitCode: exitCode}
		}
		return ErrRemoteNotFound
	} else if strings.Contains(out, "for your current branch, you must specify a branch on the command line") {
		return ErrRemoteBranchNotSpecified
	} else if strings.Contains(out, "Automatic merge failed; fix conflicts and then commit the result") {
		return ErrConflictAfterMerge
	} else if strings.Contains(out, "error: Pulling is not possible because you have unmerged files.") {
		return ErrUnmergedFiles
	} else if strings.Contains(out, "unable to resolve reference") {
		return ErrReferenceBroken
	} else if strings.Contains(out, "git config --global add user.email") {
		return ErrUserEmailNotSet
	} else if strings.Contains(out, "Permission denied (publickey)") ||
		strings.Contains(out, "Permission denied (password)") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrPermissionDenied, exitCode: exitCode}
		}
		return ErrPermissionDenied
	} else if strings.Contains(out, "would be overwritten by merge") {
		return ErrOverwrittenByMerge
	} else if strings.Contains(lowerOut, "operation timed out") ||
		strings.Contains(lowerOut, "timeout") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrNetworkTimeout, exitCode: exitCode}
		}
		return ErrNetworkTimeout
	} else if strings.Contains(lowerOut, "could not resolve hostname") ||
		strings.Contains(lowerOut, "name or service not known") ||
		strings.Contains(lowerOut, "nodename nor servname provided") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrDNSError, exitCode: exitCode}
		}
		return ErrDNSError
	} else if strings.Contains(lowerOut, "failed to connect") ||
		strings.Contains(lowerOut, "network is unreachable") ||
		strings.Contains(lowerOut, "no route to host") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrNetworkUnreachable, exitCode: exitCode}
		}
		return ErrNetworkUnreachable
	} else if strings.Contains(lowerOut, "ssl certificate problem") ||
		strings.Contains(lowerOut, "tls handshake failed") {
		if exitCode > 0 {
			return gitErrorWithExitCode{GitError: ErrSSLError, exitCode: exitCode}
		}
		return ErrSSLError
	}

	if trimmed == "" {
		return fmt.Errorf("unknown error")
	}

	return fmt.Errorf("%s", trimmed)
}
