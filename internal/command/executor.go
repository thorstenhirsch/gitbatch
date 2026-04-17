package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// Executor centralizes operation preparation so synchronous quick-mode execution
// and queued job execution share the same defaults, validation, and state
// transitions.
type Executor struct {
	repo *git.Repository
}

// NewExecutor returns an Executor for the given repository.
func NewExecutor(repo *git.Repository) *Executor {
	return &Executor{repo: repo}
}

// RunFetch executes fetch synchronously and evaluates repository state.
func (e *Executor) RunFetch(ctx context.Context, options *FetchOptions) error {
	return e.run(ctx, e.prepareFetch(options))
}

// ScheduleFetch queues fetch execution on the repository git queue.
func (e *Executor) ScheduleFetch(options *FetchOptions) error {
	return e.schedule(e.prepareFetch(options))
}

// RunPull executes pull synchronously and evaluates repository state.
func (e *Executor) RunPull(ctx context.Context, options *PullOptions, suppressSuccess bool) error {
	return e.run(ctx, e.preparePull(OperationPull, options, true, false, suppressSuccess))
}

// SchedulePull queues pull execution on the repository git queue.
func (e *Executor) SchedulePull(options *PullOptions, suppressSuccess bool) error {
	return e.schedule(e.preparePull(OperationPull, options, true, false, suppressSuccess))
}

// RunMerge executes merge synchronously and evaluates repository state.
func (e *Executor) RunMerge(ctx context.Context, options *MergeOptions) error {
	return e.run(ctx, e.prepareMerge(options))
}

// ScheduleMerge queues merge execution on the repository git queue.
func (e *Executor) ScheduleMerge(options *MergeOptions) error {
	return e.schedule(e.prepareMerge(options))
}

// RunRebase executes rebase synchronously and evaluates repository state.
func (e *Executor) RunRebase(ctx context.Context, options *PullOptions) error {
	return e.run(ctx, e.preparePull(OperationRebase, options, false, true, false))
}

// ScheduleRebase queues rebase execution on the repository git queue.
func (e *Executor) ScheduleRebase(options *PullOptions) error {
	return e.schedule(e.preparePull(OperationRebase, options, false, true, false))
}

// RunPush executes push synchronously and evaluates repository state.
func (e *Executor) RunPush(ctx context.Context, options *PushOptions, suppressSuccess bool) error {
	return e.run(ctx, e.preparePush(options, suppressSuccess))
}

// SchedulePush queues push execution on the repository git queue.
func (e *Executor) SchedulePush(options *PushOptions, suppressSuccess bool) error {
	return e.schedule(e.preparePush(options, suppressSuccess))
}

// RunCommit executes commit synchronously and evaluates repository state.
func (e *Executor) RunCommit(ctx context.Context, options *CommitOptions) error {
	return e.run(ctx, e.prepareCommit(options))
}

// ScheduleCommit queues commit execution on the repository git queue.
func (e *Executor) ScheduleCommit(options *CommitOptions) error {
	return e.schedule(e.prepareCommit(options))
}

// RunStash executes stash synchronously and evaluates repository state.
func (e *Executor) RunStash(ctx context.Context, options *StashOptions) error {
	return e.run(ctx, e.prepareStash(options))
}

// ScheduleStash queues stash execution on the repository git queue.
func (e *Executor) ScheduleStash(options *StashOptions) error {
	return e.schedule(e.prepareStash(options))
}

// RunStashPop executes stash pop synchronously and evaluates repository state.
func (e *Executor) RunStashPop(ctx context.Context, options *StashPopOptions) error {
	return e.run(ctx, e.prepareStashPop(options))
}

// ScheduleStashPop queues stash pop execution on the repository git queue.
func (e *Executor) ScheduleStashPop(options *StashPopOptions) error {
	return e.schedule(e.prepareStashPop(options))
}

// RunStashDrop executes stash drop synchronously and evaluates repository state.
func (e *Executor) RunStashDrop(ctx context.Context, options *StashDropOptions) error {
	return e.run(ctx, e.prepareStashDrop(options))
}

// ScheduleStashDrop queues stash drop execution on the repository git queue.
func (e *Executor) ScheduleStashDrop(options *StashDropOptions) error {
	return e.schedule(e.prepareStashDrop(options))
}

type executionPlan struct {
	request   *GitCommandRequest
	immediate *OperationOutcome
}

func (e *Executor) run(ctx context.Context, plan executionPlan) error {
	if err := e.validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	outcome := plan.outcome(ctx)
	ScheduleStateEvaluation(e.repo, outcome)
	return outcome.Err
}

func (e *Executor) schedule(plan executionPlan) error {
	if err := e.validate(); err != nil {
		return err
	}
	if plan.immediate != nil {
		ScheduleStateEvaluation(e.repo, *plan.immediate)
		return nil
	}
	return ScheduleGitCommand(e.repo, plan.request)
}

func (e *Executor) validate() error {
	if e == nil || e.repo == nil {
		return fmt.Errorf("repository not initialized")
	}
	if e.repo.State == nil {
		return fmt.Errorf("repository state not initialized")
	}
	return nil
}

func (p executionPlan) outcome(ctx context.Context) OperationOutcome {
	if p.immediate != nil {
		return *p.immediate
	}
	return p.request.Execute(ctx)
}

func (e *Executor) prepareFetch(options *FetchOptions) executionPlan {
	opts := normalizeFetchOptions(options, e.repo)

	if branch := e.repo.State.Branch; branch != nil {
		switch {
		case branch.Upstream == nil:
			return immediatePlan(OperationNoUpstream, "upstream not configured")
		case branch.Upstream.Reference == nil:
			msg := "upstream missing on remote"
			if upstreamName := strings.TrimSpace(branch.Upstream.Name); upstreamName != "" {
				msg = fmt.Sprintf("upstream %s missing on remote", upstreamName)
			}
			return immediatePlan(OperationFetch, msg)
		}
	}

	optsCopy := *opts
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("fetch:%s:%s", e.repo.RepoID, optsCopy.RemoteName),
		Timeout:   optsCopy.Timeout,
		Operation: OperationFetch,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := FetchWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation: OperationFetch,
				Message:   msg,
				Err:       err,
			}
		},
	})
}

func (e *Executor) preparePull(operation OperationType, options *PullOptions, ffOnly, rebase, suppressSuccess bool) executionPlan {
	if e.repo.State.Branch == nil || e.repo.State.Branch.Upstream == nil {
		return immediatePlan(operation, "upstream not set")
	}
	if e.repo.State.Remote == nil {
		return immediatePlan(operation, "remote not set")
	}

	opts := normalizePullOptions(options, e.repo, ffOnly, rebase)
	optsCopy := *opts
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("%s:%s:%s", operation, e.repo.RepoID, optsCopy.RemoteName),
		Timeout:   operationTimeout(e.repo.State.Branch.PullableCount),
		Operation: operation,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := PullWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation:       operation,
				Message:         msg,
				Err:             err,
				SuppressSuccess: suppressSuccess,
			}
		},
	})
}

func (e *Executor) prepareMerge(options *MergeOptions) executionPlan {
	if e.repo.State.Branch == nil || e.repo.State.Branch.Upstream == nil {
		return immediatePlan(OperationMerge, "upstream not set")
	}

	opts := normalizeMergeOptions(options, e.repo)
	optsCopy := *opts
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("merge:%s:%s", e.repo.RepoID, optsCopy.BranchName),
		Timeout:   operationTimeout(e.repo.State.Branch.PullableCount),
		Operation: OperationMerge,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := MergeWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation: OperationMerge,
				Message:   msg,
				Err:       err,
			}
		},
	})
}

func (e *Executor) preparePush(options *PushOptions, suppressSuccess bool) executionPlan {
	if e.repo.State.Remote == nil {
		return immediatePlan(OperationPush, "remote not set")
	}
	if e.repo.State.Branch == nil {
		return immediatePlan(OperationPush, "branch not set")
	}

	opts := normalizePushOptions(options, e.repo)
	optsCopy := *opts
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("push:%s:%s", e.repo.RepoID, optsCopy.RemoteName),
		Timeout:   operationTimeout(e.repo.State.Branch.PushableCount),
		Operation: OperationPush,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := PushWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation:       OperationPush,
				Message:         msg,
				Err:             err,
				SuppressSuccess: suppressSuccess,
			}
		},
	})
}

func (e *Executor) prepareCommit(options *CommitOptions) executionPlan {
	if options == nil || strings.TrimSpace(options.Message) == "" {
		return immediatePlan(OperationCommit, "commit options not provided")
	}

	optsCopy := *options
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("commit:%s", e.repo.RepoID),
		Timeout:   DefaultGitCommandTimeout,
		Operation: OperationCommit,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := CommitWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation: OperationCommit,
				Message:   msg,
				Err:       err,
			}
		},
	})
}

func (e *Executor) prepareStash(options *StashOptions) executionPlan {
	opts := options
	if opts == nil {
		opts = &StashOptions{}
	}

	optsCopy := *opts
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("stash:%s", e.repo.RepoID),
		Timeout:   DefaultGitCommandTimeout,
		Operation: OperationStash,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := StashWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation: OperationStash,
				Message:   msg,
				Err:       err,
			}
		},
	})
}

func (e *Executor) prepareStashPop(options *StashPopOptions) executionPlan {
	if options == nil || strings.TrimSpace(options.StashRef) == "" {
		return immediatePlan(OperationStashPop, "stash pop options not provided")
	}

	optsCopy := *options
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("stash-pop:%s", e.repo.RepoID),
		Timeout:   DefaultGitCommandTimeout,
		Operation: OperationStashPop,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := StashPopWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation: OperationStashPop,
				Message:   msg,
				Err:       err,
			}
		},
	})
}

func (e *Executor) prepareStashDrop(options *StashDropOptions) executionPlan {
	if options == nil || strings.TrimSpace(options.StashRef) == "" {
		return immediatePlan(OperationStashDrop, "stash drop options not provided")
	}

	optsCopy := *options
	return queuedPlan(&GitCommandRequest{
		Key:       fmt.Sprintf("stash-drop:%s", e.repo.RepoID),
		Timeout:   DefaultGitCommandTimeout,
		Operation: OperationStashDrop,
		Execute: func(ctx context.Context) OperationOutcome {
			msg, err := StashDropWithContext(ctx, e.repo, &optsCopy)
			return OperationOutcome{
				Operation: OperationStashDrop,
				Message:   msg,
				Err:       err,
			}
		},
	})
}

func queuedPlan(request *GitCommandRequest) executionPlan {
	return executionPlan{request: request}
}

func immediatePlan(operation OperationType, message string) executionPlan {
	return executionPlan{
		immediate: &OperationOutcome{
			Operation: operation,
			Err:       errors.New(message),
			Message:   message,
		},
	}
}

func normalizeFetchOptions(options *FetchOptions, repo *git.Repository) *FetchOptions {
	if options == nil {
		options = &FetchOptions{}
	}
	opts := *options
	if opts.RemoteName == "" {
		opts.RemoteName = repositoryRemoteName(repo)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultFetchTimeout
	}
	return &opts
}

func normalizePullOptions(options *PullOptions, repo *git.Repository, ffOnly, rebase bool) *PullOptions {
	if options == nil {
		options = &PullOptions{}
	}
	opts := *options
	if opts.RemoteName == "" {
		opts.RemoteName = repositoryRemoteName(repo)
	}
	if opts.ReferenceName == "" {
		opts.ReferenceName = git.UpstreamBranchName(repo)
	}
	if ffOnly {
		opts.FFOnly = true
	}
	if rebase {
		opts.Rebase = true
	}
	return &opts
}

func normalizeMergeOptions(options *MergeOptions, repo *git.Repository) *MergeOptions {
	if options == nil {
		options = &MergeOptions{}
	}
	opts := *options
	if opts.BranchName == "" && repo.State.Branch != nil && repo.State.Branch.Upstream != nil {
		opts.BranchName = repo.State.Branch.Upstream.Name
	}
	return &opts
}

func normalizePushOptions(options *PushOptions, repo *git.Repository) *PushOptions {
	if options == nil {
		options = &PushOptions{}
	}
	opts := *options
	if opts.RemoteName == "" {
		opts.RemoteName = repositoryRemoteName(repo)
	}
	if opts.ReferenceName == "" && repo.State.Branch != nil {
		opts.ReferenceName = repo.State.Branch.Name
	}
	return &opts
}

func repositoryRemoteName(repo *git.Repository) string {
	if repo != nil && repo.State != nil && repo.State.Remote != nil && repo.State.Remote.Name != "" {
		return repo.State.Remote.Name
	}
	return "origin"
}

func operationTimeout(countFn func() (int, bool)) time.Duration {
	if count, ok := countFn(); ok {
		return DynamicTimeout(DefaultGitCommandTimeout, count)
	}
	return DefaultGitCommandTimeout
}
