package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// GitCommandFunc describes the work executed on the git queue. The returned OperationOutcome
// will be forwarded to the state queue regardless of success or failure.
type GitCommandFunc func(ctx context.Context) OperationOutcome

const DefaultGitCommandTimeout = 10 * time.Second

// GitCommandRequest encapsulates metadata required by the git queue listener.
// It declares a timeout enforced by the queue infrastructure.
type GitCommandRequest struct {
	Key       string
	Timeout   time.Duration
	Operation OperationType
	Execute   GitCommandFunc
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
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = DefaultGitCommandTimeout
		}
		startGitOperation(r, req.Operation)
		resultCh := make(chan OperationOutcome, 1)
		done := make(chan struct{})
		go func() {
			outcome := OperationOutcome{}
			if req.Execute != nil {
				outcome = req.Execute(ctx)
			}
			if ctx.Err() != nil && outcome.Err == nil {
				outcome.Err = ctx.Err()
			}
			select {
			case resultCh <- outcome:
			case <-done:
			}
		}()
		select {
		case outcome := <-resultCh:
			close(done)
			if outcome.Operation == "" {
				outcome.Operation = req.Operation
			}
			ScheduleStateEvaluation(r, outcome)
		case <-time.After(timeout):
			close(done)
			op := req.Operation
			if op == "" {
				op = OperationGit
			}
			err := fmt.Errorf("%s command timed out after %s", op, timeout)
			ScheduleStateEvaluation(r, OperationOutcome{
				Operation: op,
				Err:       err,
				Message:   "git command timed out",
			})
			select {
			case <-resultCh:
			default:
			}
		}
		return nil
	})
}

func startGitOperation(r *git.Repository, operation OperationType) {
	if r == nil {
		return
	}
	message := "running git command..."
	switch operation {
	case OperationFetch:
		message = "fetching..."
	case OperationPull:
		message = "pulling..."
	case OperationMerge:
		message = "merging..."
	case OperationRebase:
		message = "rebasing..."
	case OperationPush:
		message = "pushing..."
	default:
		if op := strings.TrimSpace(string(operation)); op != "" && operation != OperationGit {
			message = fmt.Sprintf("%s...", strings.ToLower(op))
		}
	}
	setRepositoryStatus(r, git.Working, message)
}

func init() {
	git.RegisterRepositoryHook(AttachGitCommandWorker)
}
