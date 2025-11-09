package command

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func init() {
	git.RegisterRepositoryHook(AttachStateEvaluator)
}

// OperationType identifies the job or action that updated a repository.
type OperationType string

const (
	OperationFetch      OperationType = "fetch"
	OperationPull       OperationType = "pull"
	OperationMerge      OperationType = "merge"
	OperationRebase     OperationType = "rebase"
	OperationPush       OperationType = "push"
	OperationRefresh    OperationType = "refresh"
	OperationGit        OperationType = "git"
	OperationStateProbe OperationType = "state-probe"
)

// OperationOutcome captures the result of an operation for state evaluation.
type OperationOutcome struct {
	Operation           OperationType
	Err                 error
	Message             string
	SuppressSuccess     bool
	RecoverableOverride *bool
}

// isGitFatalError checks if an error is a git fatal error (exit code 128).
// These errors indicate serious problems like network failures, permission issues,
// or repository corruption and should be treated as critical, not recoverable.
func isGitFatalError(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			// Exit code 128 is git's fatal error code
			// This includes: network errors, permission denied, repository not found, etc.
			return status.ExitStatus() == 128
		}
	}
	return false
}

// EvaluateRepositoryState centralises repository state transitions after an operation.
// It is invoked after auto-fetch, queued jobs, and lazygit refreshes to ensure
// consistent clean/dirty and error handling across the application.
func EvaluateRepositoryState(r *git.Repository, outcome OperationOutcome) {
	if r == nil || r.State == nil {
		return
	}

	if outcome.Operation == OperationStateProbe {
		// Check if this is an initial state probe request (no message/result yet)
		// vs. a completion (has message or error from the async operation)
		if outcome.Err == nil && outcome.Message == "" {
			// Initial request - schedule the async probe
			handleStateProbe(r)
			return
		}
		// This is a completion result from the async state probe.
		// Handle errors normally, or apply success state if no error.
		if outcome.Err == nil {
			applySuccessState(r, outcome)
			applyCleanliness(r)
			return
		}
		// Fall through to error handling below
	}

	if outcome.Err != nil {
		recoverable := false
		if outcome.RecoverableOverride != nil {
			recoverable = *outcome.RecoverableOverride
		} else {
			recoverable = gerr.IsRecoverable(outcome.Err)
		}
		message := strings.TrimSpace(outcome.Message)
		if message == "" {
			message = git.NormalizeGitErrorMessage(outcome.Err.Error())
		}
		if recoverable {
			r.MarkRecoverableError(message)
		} else {
			r.MarkCriticalError(message)
		}
		return
	}

	applySuccessState(r, outcome)
	applyCleanliness(r)
}

// AttachStateEvaluator wires repository events to the state evaluator.
func AttachStateEvaluator(r *git.Repository) {
	if r == nil {
		return
	}
	r.On(git.RepositoryEvaluationRequested, func(event *git.RepositoryEvent) error {
		if event == nil {
			return nil
		}
		var outcome OperationOutcome
		switch payload := event.Data.(type) {
		case OperationOutcome:
			outcome = payload
		case *OperationOutcome:
			if payload != nil {
				outcome = *payload
			}
		default:
			return nil
		}
		prev := snapshotState(r)
		EvaluateRepositoryState(r, outcome)
		// Only schedule a refresh if the operation succeeded.
		// Refreshing after an error would overwrite the error state.
		if outcome.Err == nil && outcome.Operation != OperationRefresh && outcome.Operation != OperationStateProbe && stateChanged(prev, r) {
			_ = ScheduleRepositoryRefresh(r, nil)
		}
		return nil
	})
}

// ScheduleStateEvaluation emits an event-driven request to recompute repository state.
func ScheduleStateEvaluation(r *git.Repository, outcome OperationOutcome) {
	if r == nil {
		return
	}
	_ = r.Publish(git.RepositoryEvaluationRequested, outcome)
}

// ScheduleRepositoryRefresh emits an event-driven request to refresh repository metadata.
// When outcome is non-nil, it will be forwarded to the refresh listener so the resulting
// state evaluation can apply the provided context after the refresh completes.
func ScheduleRepositoryRefresh(r *git.Repository, outcome *OperationOutcome) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	return r.Publish(git.RepositoryRefreshRequested, outcome)
}

type stateSnapshot struct {
	status      git.WorkStatus
	message     string
	recoverable bool
}

func snapshotState(r *git.Repository) stateSnapshot {
	if r == nil || r.State == nil {
		return stateSnapshot{}
	}
	return stateSnapshot{
		status:      r.WorkStatus(),
		message:     r.State.Message,
		recoverable: r.State.RecoverableError,
	}
}

func stateChanged(prev stateSnapshot, r *git.Repository) bool {
	if r == nil || r.State == nil {
		return false
	}
	if prev.status != r.WorkStatus() {
		return true
	}
	if prev.message != r.State.Message {
		return true
	}
	if prev.recoverable != r.State.RecoverableError {
		return true
	}
	return false
}

func handleStateProbe(r *git.Repository) {
	if r == nil || r.State == nil {
		return
	}

	setRepositoryStatus(r, git.Pending, "waiting")

	branch := r.State.Branch
	if branch == nil {
		r.MarkRecoverableError("branch not set")
		return
	}

	upstream := branch.Upstream
	if upstream == nil {
		r.MarkRecoverableError("upstream not configured")
		return
	}

	remoteName, remoteBranch := resolveUpstreamParts(r, branch)
	if remoteName == "" || remoteBranch == "" {
		r.MarkRecoverableError("upstream not configured")
		return
	}

	// Schedule the upstream verification and fetch asynchronously via the git queue
	// to avoid blocking the TUI
	if err := scheduleUpstreamVerificationAndFetch(r, remoteName, remoteBranch); err != nil {
		r.MarkRecoverableError(fmt.Sprintf("unable to schedule verification: %v", err))
		return
	}
}

// scheduleUpstreamVerificationAndFetch asynchronously verifies the upstream exists and then fetches.
// This prevents blocking the TUI during initial state probe operations.
func scheduleUpstreamVerificationAndFetch(r *git.Repository, remoteName, remoteBranch string) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	if remoteName == "" || remoteBranch == "" {
		return fmt.Errorf("remote or branch missing")
	}

	req := &GitCommandRequest{
		Key:       fmt.Sprintf("state-probe:%s:%s:%s", r.RepoID, remoteName, remoteBranch),
		Timeout:   DefaultFetchTimeout,
		Operation: OperationStateProbe,
		Execute: func(ctx context.Context) OperationOutcome {
			// First verify the upstream exists
			exists, err := upstreamExistsOnRemoteWithContext(ctx, r, remoteName, remoteBranch)
			if err != nil {
				// Fatal errors (exit 128) should be critical, not recoverable
				// These include: network failures, permission denied, repo not found
				recoverable := !isGitFatalError(err)
				return OperationOutcome{
					Operation:           OperationStateProbe,
					Err:                 err,
					Message:             fmt.Sprintf("unable to verify upstream: %v", err),
					RecoverableOverride: &recoverable,
				}
			}
			if !exists {
				// Missing upstream is recoverable - user can configure it
				recoverable := true
				return OperationOutcome{
					Operation:           OperationStateProbe,
					Err:                 fmt.Errorf("upstream %s missing on remote", remoteName+"/"+remoteBranch),
					Message:             fmt.Sprintf("upstream %s missing on remote", remoteName+"/"+remoteBranch),
					RecoverableOverride: &recoverable,
				}
			}

			// Now fetch from the remote
			opts := FetchOptions{
				RemoteName:  remoteName,
				CommandMode: ModeLegacy,
				Timeout:     DefaultFetchTimeout,
			}
			msg, err := FetchWithContext(ctx, r, &opts)
			// Return OperationStateProbe (not OperationFetch) to avoid triggering
			// a refresh cycle. The state probe is the initial evaluation and should
			// complete without scheduling additional operations.
			//
			// Fatal errors (exit 128) during fetch should be critical.
			// Other fetch errors are generally recoverable.
			var recoverableOverride *bool
			if err != nil {
				recoverable := !isGitFatalError(err)
				recoverableOverride = &recoverable
			}
			return OperationOutcome{
				Operation:           OperationStateProbe,
				Message:             msg,
				Err:                 err,
				RecoverableOverride: recoverableOverride,
			}
		},
	}
	return ScheduleGitCommand(r, req)
}

// upstreamExistsOnRemoteWithContext is a context-aware version of upstreamExistsOnRemote.
func upstreamExistsOnRemoteWithContext(ctx context.Context, r *git.Repository, remoteName, branchName string) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("repository not initialized")
	}
	if remoteName == "" || branchName == "" {
		return false, fmt.Errorf("remote or branch missing")
	}

	branchRef := branchName
	if !strings.HasPrefix(branchRef, "refs/") {
		branchRef = "refs/heads/" + branchRef
	}

	args := []string{"ls-remote", "--heads", remoteName, branchRef}
	out, err := RunWithContextTimeout(ctx, r.AbsPath, "git", args, DefaultFetchTimeout)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func applySuccessState(r *git.Repository, outcome OperationOutcome) {
	if r.State == nil {
		return
	}

	// Reset recoverable flag on success paths.
	r.State.RecoverableError = false

	message := strings.TrimSpace(outcome.Message)
	prevMessage := ""
	if r.State != nil {
		prevMessage = r.State.Message
	}
	statusChanged := false
	notified := false

	switch outcome.Operation {
	case OperationFetch:
		prevStatus := r.WorkStatus()
		r.SetWorkStatus(git.Available)
		statusChanged = prevStatus != r.WorkStatus()
		r.State.Message = message
	case OperationPull:
		if outcome.SuppressSuccess {
			prevStatus := r.WorkStatus()
			r.SetWorkStatus(git.Available)
			statusChanged = prevStatus != r.WorkStatus()
		} else {
			prevStatus := r.WorkStatus()
			r.SetWorkStatus(git.Success)
			statusChanged = prevStatus != r.WorkStatus()
		}
		if message == "" {
			r.State.Message = "pull completed"
		} else {
			r.State.Message = message
		}
	case OperationMerge:
		prevStatus := r.WorkStatus()
		r.SetWorkStatus(git.Success)
		statusChanged = prevStatus != r.WorkStatus()
		if message == "" {
			r.State.Message = "merge completed"
		} else {
			r.State.Message = message
		}
	case OperationRebase:
		prevStatus := r.WorkStatus()
		r.SetWorkStatus(git.Success)
		statusChanged = prevStatus != r.WorkStatus()
		if message == "" {
			r.State.Message = "rebase completed"
		} else {
			r.State.Message = message
		}
	case OperationPush:
		if outcome.SuppressSuccess {
			prevStatus := r.WorkStatus()
			r.SetWorkStatus(git.Available)
			statusChanged = prevStatus != r.WorkStatus()
		} else {
			prevStatus := r.WorkStatus()
			r.SetWorkStatus(git.Success)
			statusChanged = prevStatus != r.WorkStatus()
		}
		if message == "" {
			r.State.Message = "push completed"
		} else {
			r.State.Message = message
		}
	case OperationRefresh:
		prevStatus := r.WorkStatus()
		r.State.Message = message
		if prevStatus != git.Available {
			r.SetWorkStatus(git.Available)
			statusChanged = true
		} else if strings.TrimSpace(prevMessage) != strings.TrimSpace(message) {
			r.NotifyRepositoryUpdated()
			notified = true
		}
	case OperationStateProbe:
		// For state probes, don't change the status yet - let applyCleanliness
		// handle the final status determination after checking for conflicts.
		// Keep status as Working/Pending so spinner continues during cleanliness check.
		r.State.Message = message
		if strings.TrimSpace(prevMessage) != strings.TrimSpace(message) {
			r.NotifyRepositoryUpdated()
			notified = true
		}
	default:
		prevStatus := r.WorkStatus()
		r.SetWorkStatus(git.Available)
		statusChanged = prevStatus != r.WorkStatus()
		r.State.Message = message
	}

	if !statusChanged && !notified && strings.TrimSpace(prevMessage) != strings.TrimSpace(r.State.Message) {
		r.NotifyRepositoryUpdated()
	}
}

func applyCleanliness(r *git.Repository) {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return
	}

	branch := r.State.Branch

	// Check if the working tree is clean according to git
	workingTreeClean := r.IsClean()

	if branch.HasIncomingCommits() {
		upstream := branch.Upstream
		if upstream == nil || upstream.Name == "" {
			r.MarkRecoverableError("upstream not configured")
			return
		}

		// If the working tree is clean (no local changes), mark the repo as clean.
		// With no local changes, there can be no merge conflicts with incoming commits.
		if workingTreeClean {
			r.MarkClean()
			if r.WorkStatus() != git.Available {
				r.SetWorkStatus(git.Available)
			}
			return
		}

		// Working tree is NOT clean (has local changes) AND there are incoming commits.
		// We need to check if the local changes would conflict with incoming commits.
		// Use fast-forward dry-run to determine this.
		mergeArg := upstreamMergeArgument(upstream)
		if mergeArg == "" {
			r.MarkRecoverableError("upstream not configured")
			return
		}

		// Set status to Working so spinner shows during the fast-forward check
		prevStatus := r.WorkStatus()
		if prevStatus != git.Working {
			setRepositoryStatus(r, git.Working, "checking for conflicts...")
		}

		succeeds, err := fastForwardDryRunSucceeds(r, mergeArg)
		if err != nil {
			r.MarkRecoverableError(fmt.Sprintf("unable to verify fast-forward: %v", err))
			return
		}

		// Even though git reports "working tree NOT clean", if fast-forward
		// succeeds, the local changes don't conflict with incoming commits.
		if succeeds {
			r.MarkClean()
		} else {
			r.MarkDirty()
		}
		if r.WorkStatus() != git.Available {
			r.SetWorkStatus(git.Available)
		}
		return
	}

	// No incoming commits - the branch is up-to-date with upstream.
	r.MarkClean()
	if r.WorkStatus() != git.Available {
		r.SetWorkStatus(git.Available)
	}
}

func resolveUpstreamParts(r *git.Repository, branch *git.Branch) (string, string) {
	if branch == nil || branch.Upstream == nil {
		return "", ""
	}

	var remoteName string
	remoteBranch := branch.Name

	if branch.Upstream.Reference != nil {
		short := branch.Upstream.Reference.Name().Short()
		if parts := strings.SplitN(short, "/", 2); len(parts) == 2 {
			remoteName = parts[0]
			remoteBranch = parts[1]
		}
	} else if branch.Upstream.Name != "" {
		short := branch.Upstream.Name
		short = strings.TrimPrefix(short, "refs/remotes/")
		if parts := strings.SplitN(short, "/", 2); len(parts) == 2 {
			remoteName = parts[0]
			remoteBranch = parts[1]
		}
	}

	if strings.EqualFold(remoteBranch, "HEAD") {
		remoteBranch = branch.Name
	}
	if remoteName == "" && r.State != nil && r.State.Remote != nil {
		remoteName = r.State.Remote.Name
	}

	return remoteName, remoteBranch
}

func scheduleFetchCommand(r *git.Repository, remoteName string) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	if remoteName == "" {
		remoteName = defaultRemoteName(r)
	}
	if remoteName == "" {
		return fmt.Errorf("remote not set")
	}
	opts := FetchOptions{
		RemoteName:  remoteName,
		CommandMode: ModeLegacy,
		Timeout:     DefaultFetchTimeout,
	}
	optsCopy := opts
	req := &GitCommandRequest{
		Key:       fmt.Sprintf("fetch:%s:%s", r.RepoID, optsCopy.RemoteName),
		Timeout:   optsCopy.Timeout,
		Operation: OperationFetch,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := FetchWithContext(ctx, r, &optsCopy)
			return OperationOutcome{
				Operation: OperationFetch,
				Message:   msg,
				Err:       err,
			}
		},
	}
	return ScheduleGitCommand(r, req)
}

func defaultRemoteName(r *git.Repository) string {
	if r != nil && r.State != nil && r.State.Remote != nil && r.State.Remote.Name != "" {
		return r.State.Remote.Name
	}
	return "origin"
}

func fastForwardDryRunSucceeds(r *git.Repository, mergeArg string) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("repository not initialized")
	}
	if mergeArg == "" {
		return false, fmt.Errorf("upstream reference not set")
	}
	// Try a merge with --no-commit --no-ff to test if merge would succeed
	// --no-commit prevents actually committing
	// We use merge-tree if available (git 2.38+), otherwise use error messages
	out, err := Run(r.AbsPath, "git", []string{"merge-tree", "--write-tree", "HEAD", mergeArg})
	if err == nil {
		// merge-tree succeeded, meaning no conflicts
		return true, nil
	}

	// merge-tree not available or failed, fall back to checking for specific error patterns
	// Try the merge and check the error message
	out, err = Run(r.AbsPath, "git", []string{"merge", "--ff-only", "--no-commit", mergeArg})
	if err == nil {
		// Merge would succeed, but we need to abort it since we used --no-commit
		_, _ = Run(r.AbsPath, "git", []string{"merge", "--abort"})
		return true, nil
	}

	// Check if the error is because of uncommitted local changes that would be overwritten.
	// If git says "would be overwritten by merge", it means the fast-forward
	// itself would work, but git won't execute it due to uncommitted changes.
	// This is NOT a merge conflict - treat it as "fast-forward would succeed".
	if strings.Contains(out, "would be overwritten by merge") ||
		strings.Contains(out, "would be overwritten by checkout") {
		return true, nil
	}

	// Check for "Cannot merge" which indicates actual conflicts
	if strings.Contains(out, "Cannot merge") ||
		strings.Contains(out, "CONFLICT") {
		return false, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Any other exit error means fast-forward would fail (merge conflict)
		return false, nil
	}
	return false, err
}

func setRepositoryStatus(r *git.Repository, status git.WorkStatus, message string) {
	if r == nil || r.State == nil {
		return
	}
	prevStatus := r.WorkStatus()
	prevMessage := strings.TrimSpace(r.State.Message)
	trimmed := strings.TrimSpace(message)
	r.State.Message = message
	if prevStatus != status {
		r.SetWorkStatus(status)
		return
	}
	if prevMessage != trimmed {
		r.NotifyRepositoryUpdated()
	}
}

func upstreamMergeArgument(upstream *git.RemoteBranch) string {
	if upstream == nil {
		return ""
	}
	if name := strings.TrimSpace(upstream.Name); name != "" {
		return name
	}
	if upstream.Reference != nil {
		return upstream.Reference.Name().String()
	}
	return ""
}
