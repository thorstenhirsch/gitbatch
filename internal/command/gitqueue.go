package command

import (
	"context"
	"fmt"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// GitCommandFunc describes the work executed on the git queue. The returned OperationOutcome
// will be forwarded to the state queue regardless of success or failure.
type GitCommandFunc func(ctx context.Context) OperationOutcome

// GitCommandRequest encapsulates metadata required by the git queue listener.
// It provides deduplication keys for debounce handling and declares a timeout
// enforced by the queue infrastructure.
type GitCommandRequest struct {
	Key     string
	Timeout time.Duration
	Execute GitCommandFunc
}

// DebounceKey satisfies git.DebounceKeyProvider for queue deduplication.
func (r GitCommandRequest) DebounceKey() string {
	return r.Key
}

// TimeoutDuration satisfies git.TimeoutProvider so the queue can derive context deadlines.
func (r GitCommandRequest) TimeoutDuration() time.Duration {
	return r.Timeout
}

// ScheduleGitCommand publishes a request to the repository git queue.
func ScheduleGitCommand(repo *git.Repository, request *GitCommandRequest) error {
	if repo == nil {
		return fmt.Errorf("repository not initialized")
	}
	if request == nil || request.Execute == nil {
		return fmt.Errorf("invalid git command request")
	}
	if request.Key == "" {
		return fmt.Errorf("git command request key required")
	}
	return repo.Publish(git.RepositoryGitCommandRequested, request)
}

// AttachGitCommandWorker registers the git queue listener responsible for executing
// scheduled git commands and forwarding their outcomes to the state queue.
func AttachGitCommandWorker(r *git.Repository) {
	if r == nil {
		return
	}
	r.On(git.RepositoryGitCommandRequested, func(event *git.RepositoryEvent) error {
		req, ok := event.Data.(*GitCommandRequest)
		if !ok || req == nil {
			return fmt.Errorf("unexpected git command payload: %T", event.Data)
		}
		ctx := event.Context
		if ctx == nil {
			ctx = context.Background()
		}
		outcome := OperationOutcome{}
		if req.Execute != nil {
			outcome = req.Execute(ctx)
		}
		if ctx.Err() != nil && outcome.Err == nil {
			outcome.Err = ctx.Err()
		}
		ScheduleStateEvaluation(r, outcome)
		return nil
	})
}

func init() {
	git.RegisterRepositoryHook(AttachGitCommandWorker)
}
