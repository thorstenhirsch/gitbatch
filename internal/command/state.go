package command

import (
	"fmt"
	"strings"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func init() {
	git.RegisterRepositoryHook(AttachStateEvaluator)
}

// OperationType identifies the job or action that updated a repository.
type OperationType string

const (
	OperationFetch   OperationType = "fetch"
	OperationPull    OperationType = "pull"
	OperationMerge   OperationType = "merge"
	OperationRebase  OperationType = "rebase"
	OperationPush    OperationType = "push"
	OperationRefresh OperationType = "refresh"
)

// OperationOutcome captures the result of an operation for state evaluation.
type OperationOutcome struct {
	Operation           OperationType
	Err                 error
	Message             string
	SuppressSuccess     bool
	RecoverableOverride *bool
}

// EvaluateRepositoryState centralises repository state transitions after an operation.
// It is invoked after auto-fetch, queued jobs, and lazygit refreshes to ensure
// consistent clean/dirty and error handling across the application.
func EvaluateRepositoryState(r *git.Repository, outcome OperationOutcome) {
	if r == nil || r.State == nil {
		return
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
		EvaluateRepositoryState(r, outcome)
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

func applySuccessState(r *git.Repository, outcome OperationOutcome) {
	if r.State == nil {
		return
	}

	// Reset recoverable flag on success paths.
	r.State.RecoverableError = false

	message := strings.TrimSpace(outcome.Message)

	switch outcome.Operation {
	case OperationFetch:
		r.SetWorkStatus(git.Available)
		r.State.Message = message
	case OperationPull:
		if outcome.SuppressSuccess {
			r.SetWorkStatus(git.Available)
		} else {
			r.SetWorkStatus(git.Success)
		}
		if message == "" {
			r.State.Message = "pull completed"
		} else {
			r.State.Message = message
		}
	case OperationMerge:
		r.SetWorkStatus(git.Success)
		if message == "" {
			r.State.Message = "merge completed"
		} else {
			r.State.Message = message
		}
	case OperationRebase:
		r.SetWorkStatus(git.Success)
		if message == "" {
			r.State.Message = "rebase completed"
		} else {
			r.State.Message = message
		}
	case OperationPush:
		if outcome.SuppressSuccess {
			r.SetWorkStatus(git.Available)
		} else {
			r.SetWorkStatus(git.Success)
		}
		if message == "" {
			r.State.Message = "push completed"
		} else {
			r.State.Message = message
		}
	case OperationRefresh:
		r.SetWorkStatus(git.Available)
		r.State.Message = message
	default:
		r.SetWorkStatus(git.Available)
		r.State.Message = message
	}
}

func applyCleanliness(r *git.Repository) {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return
	}

	branch := r.State.Branch

	if branch.HasIncomingCommits() {
		upstream := branch.Upstream
		if upstream == nil || upstream.Name == "" {
			r.MarkRecoverableError("upstream not configured")
			return
		}
		succeeds, err := fastForwardDryRunSucceeds(r, upstream)
		if err != nil {
			r.MarkRecoverableError(fmt.Sprintf("unable to verify fast-forward: %v", err))
			return
		}
		if succeeds {
			r.MarkClean()
			return
		}
		// Remote has incoming commits with local divergence; keep dirty state to surface conflicts.
		r.MarkDirty()
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

	exists, err := upstreamExistsOnRemote(r, remoteName, remoteBranch)
	if err != nil {
		r.MarkRecoverableError(fmt.Sprintf("unable to verify upstream: %v", err))
		return
	}
	if !exists {
		r.MarkRecoverableError(fmt.Sprintf("upstream %s missing on remote", remoteName+"/"+remoteBranch))
		return
	}

	r.MarkClean()
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

func upstreamExistsOnRemote(r *git.Repository, remoteName, branchName string) (bool, error) {
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
	out, err := RunWithTimeout(r.AbsPath, "git", args, DefaultFetchTimeout)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func fastForwardDryRunSucceeds(r *git.Repository, upstream *git.RemoteBranch) (bool, error) {
	if r == nil || upstream == nil {
		return false, fmt.Errorf("repository or upstream missing")
	}

	upstreamRef := upstream.Name
	if upstreamRef == "" && upstream.Reference != nil {
		upstreamRef = upstream.Reference.Name().String()
	}
	if upstreamRef == "" {
		return false, fmt.Errorf("upstream reference not set")
	}

	headHash, err := Run(r.AbsPath, "git", []string{"rev-parse", "HEAD"})
	if err != nil {
		return false, err
	}

	mergeBase, err := Run(r.AbsPath, "git", []string{"merge-base", "HEAD", upstreamRef})
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(headHash) != "" && strings.TrimSpace(headHash) == strings.TrimSpace(mergeBase), nil
}
