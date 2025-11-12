package errors

import (
	stdErrors "errors"
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
	ErrUserEmailNotSet GitError = ("user email not configured")
	// ErrUnclassified is unconsidered error type
	ErrUnclassified GitError = ("unclassified error")
	// NoErrIterationHalted is thrown for catching stops in interators
	NoErrIterationHalted GitError = ("iteration halted")
)

func (e GitError) Error() string {
	return string(e)
}

// ParseGitError takes git output as an input and tries to find some meaningful
// errors can be used by the app
func ParseGitError(out string, err error) error {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" && err != nil {
		trimmed = strings.TrimSpace(err.Error())
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
		strings.Contains(lowerTrimmed, "fatal: authentication") {
		return ErrAuthenticationRequired
	}

	if strings.Contains(out, "error: Your local changes to the following files would be overwritten by merge") {
		return ErrMergeAbortedTryCommit
	} else if strings.Contains(out, "ERROR: Repository not found") {
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
	} else if strings.Contains(out, "Permission denied (publickey)") {
		return ErrPermissionDenied
	} else if strings.Contains(out, "would be overwritten by merge") {
		return ErrOverwrittenByMerge
	}

	if trimmed == "" {
		return fmt.Errorf("unknown error")
	}

	return fmt.Errorf("%s", trimmed)
}

// IsRecoverable reports whether the provided git error represents a recoverable state.
func IsRecoverable(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case stdErrors.Is(err, ErrRemoteBranchNotSpecified):
		return true
	case stdErrors.Is(err, ErrCouldNotFindRemoteRef):
		return true
	case stdErrors.Is(err, ErrReferenceBroken):
		return true
	}
	lowered := strings.ToLower(strings.TrimSpace(err.Error()))
	if lowered == "" {
		return false
	}
	return strings.Contains(lowered, "upstream is gone")
}

// RequiresCredentials reports whether the provided error indicates that
// authentication credentials are required to complete the operation.
func RequiresCredentials(err error) bool {
	if err == nil {
		return false
	}
	if stdErrors.Is(err, ErrAuthenticationRequired) {
		return true
	}
	if stdErrors.Is(err, ErrPermissionDenied) {
		return true
	}
	if stdErrors.Is(err, ErrAuthorizationFailed) {
		return true
	}
	lowered := strings.ToLower(strings.TrimSpace(err.Error()))
	if lowered == "" {
		return false
	}
	// Check for various authentication error patterns
	return strings.Contains(lowered, "authentication required") ||
		strings.Contains(lowered, "authentication failed") ||
		strings.Contains(lowered, "permission denied") ||
		strings.Contains(lowered, "authorization failed") ||
		strings.Contains(lowered, "could not read username") ||
		strings.Contains(lowered, "could not read password") ||
		strings.Contains(lowered, "invalid username or password") ||
		strings.Contains(lowered, "http basic: access denied") ||
		strings.Contains(lowered, "fatal: authentication") ||
		strings.Contains(lowered, "401") ||
		strings.Contains(lowered, "403 forbidden")
}
