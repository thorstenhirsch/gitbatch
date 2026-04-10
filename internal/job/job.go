package job

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// Job relates the type of the operation and the entity
type Job struct {
	// JobType is to select operation type that will be applied to repository
	JobType Type
	// Repository points to the repository that will be used for operation
	Repository *git.Repository
	// Options is a placeholder for operation options
	Options interface{}
}

// Type is the a git operation supported
type Type string

const (
	// FetchJob is wrapper of git fetch command
	FetchJob Type = "fetch"

	// PullJob is wrapper of git pull command
	PullJob Type = "pull"

	// MergeJob is wrapper of git merge command
	MergeJob Type = "merge"

	// RebaseJob is wrapper of git pull --rebase command
	RebaseJob Type = "rebase"

	// PushJob is wrapper of git push command
	PushJob Type = "push"
)

// PullJobConfig wraps pull options with queue behaviour flags.
type PullJobConfig struct {
	Options         *command.PullOptions
	SuppressSuccess bool
}

// PushJobConfig wraps push options with queue behaviour flags.
type PushJobConfig struct {
	Options         *command.PushOptions
	SuppressSuccess bool
	AllowForce      bool
}

// Start executes the job by scheduling the appropriate git command.
// The job will be processed asynchronously by the git queue.
func (j *Job) Start() error {
	if j == nil || j.Repository == nil {
		return fmt.Errorf("job or repository not initialized")
	}
	if j.Repository.State == nil {
		return fmt.Errorf("repository state not initialized")
	}
	j.Repository.SetWorkStatus(git.Working)
	// TODO: Better implementation required
	switch mode := j.JobType; mode {
	case FetchJob:
		if j.Repository.State != nil {
			j.Repository.State.Message = "fetching.."
		}
		var opts *command.FetchOptions
		switch cfg := j.Options.(type) {
		case nil:
			opts = nil
		case *command.FetchOptions:
			opts = cfg
		case command.FetchOptions:
			opts = &cfg
		default:
			opts = nil
		}
		if opts == nil {
			remoteName := "origin"
			if j.Repository.State != nil && j.Repository.State.Remote != nil && j.Repository.State.Remote.Name != "" {
				remoteName = j.Repository.State.Remote.Name
			}
			opts = &command.FetchOptions{
				RemoteName: remoteName,
				Timeout:    command.DefaultFetchTimeout,
			}
		}
		if branch := j.Repository.State.Branch; branch != nil {
			if branch.Upstream == nil {
				command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
					Operation: command.OperationNoUpstream,
					Message:   "upstream not configured",
				})
				return nil
			} else if branch.Upstream.Reference == nil {
				upstreamName := strings.TrimSpace(branch.Upstream.Name)
				msg := "upstream missing on remote"
				if upstreamName != "" {
					msg = fmt.Sprintf("upstream %s missing on remote", upstreamName)
				}
				command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
					Operation: command.OperationFetch,
					Err:       errors.New(msg),
					Message:   msg,
				})
				return nil
			}
		}
		optsCopy := *opts
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("fetch:%s:%s", j.Repository.RepoID, optsCopy.RemoteName),
			Timeout:   optsCopy.Timeout,
			Operation: command.OperationFetch,
			Execute: func(ctx context.Context) command.OperationOutcome {
				msg, err := command.FetchWithContext(ctx, j.Repository, &optsCopy)
				return command.OperationOutcome{
					Operation: command.OperationFetch,
					Message:   msg,
					Err:       err,
				}
			},
		}
		if err := command.ScheduleGitCommand(j.Repository, req); err != nil {
			return err
		}
	case PullJob:
		if j.Repository.State != nil {
			j.Repository.State.Message = "pulling.."
		}
		var (
			opts     *command.PullOptions
			suppress bool
		)
		switch cfg := j.Options.(type) {
		case nil:
			opts = &command.PullOptions{}
		case *command.PullOptions:
			opts = cfg
		case PullJobConfig:
			opts = cfg.Options
			suppress = cfg.SuppressSuccess
		case *PullJobConfig:
			opts = cfg.Options
			suppress = cfg.SuppressSuccess
		default:
			opts = &command.PullOptions{}
		}
		if opts == nil {
			opts = &command.PullOptions{}
		}
		if j.Repository.State == nil || j.Repository.State.Branch == nil || j.Repository.State.Branch.Upstream == nil {
			msg := "upstream not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationPull,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		if j.Repository.State.Remote == nil {
			msg := "remote not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationPull,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		opts = ensurePullOptions(opts, j.Repository, true, false)
		optsCopy := *opts
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("pull:%s:%s", j.Repository.RepoID, optsCopy.RemoteName),
			Timeout:   dynamicTimeout(j.Repository.State.Branch.PullableCount),
			Operation: command.OperationPull,
			Execute: func(ctx context.Context) command.OperationOutcome {
				msg, err := command.PullWithContext(ctx, j.Repository, &optsCopy)
				return command.OperationOutcome{
					Operation:       command.OperationPull,
					Message:         msg,
					Err:             err,
					SuppressSuccess: suppress,
				}
			},
		}
		if err := command.ScheduleGitCommand(j.Repository, req); err != nil {
			return err
		}
	case MergeJob:
		if j.Repository.State != nil {
			j.Repository.State.Message = "merging.."
		}
		if j.Repository.State == nil || j.Repository.State.Branch == nil || j.Repository.State.Branch.Upstream == nil {
			msg := "upstream not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationMerge,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		optsCopy := command.MergeOptions{BranchName: j.Repository.State.Branch.Upstream.Name}
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("merge:%s:%s", j.Repository.RepoID, optsCopy.BranchName),
			Timeout:   dynamicTimeout(j.Repository.State.Branch.PullableCount),
			Operation: command.OperationMerge,
			Execute: func(ctx context.Context) command.OperationOutcome {
				msg, err := command.MergeWithContext(ctx, j.Repository, &optsCopy)
				return command.OperationOutcome{
					Operation: command.OperationMerge,
					Message:   msg,
					Err:       err,
				}
			},
		}
		if err := command.ScheduleGitCommand(j.Repository, req); err != nil {
			return err
		}
	case RebaseJob:
		if j.Repository.State != nil {
			j.Repository.State.Message = "rebasing.."
		}
		if j.Repository.State == nil || j.Repository.State.Branch == nil || j.Repository.State.Branch.Upstream == nil {
			msg := "upstream not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationRebase,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		if j.Repository.State.Remote == nil {
			msg := "remote not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationRebase,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		var opts *command.PullOptions
		switch cfg := j.Options.(type) {
		case nil:
			opts = &command.PullOptions{}
		case *command.PullOptions:
			opts = cfg
		case command.PullOptions:
			opts = &cfg
		case PullJobConfig:
			opts = cfg.Options
		case *PullJobConfig:
			opts = cfg.Options
		default:
			opts = &command.PullOptions{}
		}
		if opts == nil {
			opts = &command.PullOptions{}
		}
		opts = ensurePullOptions(opts, j.Repository, false, true)
		optsCopy := *opts
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("rebase:%s:%s", j.Repository.RepoID, optsCopy.RemoteName),
			Timeout:   dynamicTimeout(j.Repository.State.Branch.PullableCount),
			Operation: command.OperationRebase,
			Execute: func(ctx context.Context) command.OperationOutcome {
				msg, err := command.PullWithContext(ctx, j.Repository, &optsCopy)
				return command.OperationOutcome{
					Operation: command.OperationRebase,
					Message:   msg,
					Err:       err,
				}
			},
		}
		if err := command.ScheduleGitCommand(j.Repository, req); err != nil {
			return err
		}
	case PushJob:
		if j.Repository.State != nil {
			j.Repository.State.Message = "pushing.."
		}
		if j.Repository.State == nil || j.Repository.State.Remote == nil {
			msg := "remote not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationPush,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		if j.Repository.State.Branch == nil {
			msg := "branch not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation: command.OperationPush,
				Err:       errors.New(msg),
				Message:   msg,
			})
			return nil
		}
		var (
			opts     *command.PushOptions
			suppress bool
		)
		switch cfg := j.Options.(type) {
		case nil:
			opts = &command.PushOptions{}
		case *command.PushOptions:
			opts = cfg
		case PushJobConfig:
			opts = cfg.Options
			suppress = cfg.SuppressSuccess
		case *PushJobConfig:
			opts = cfg.Options
			suppress = cfg.SuppressSuccess
		default:
			opts = &command.PushOptions{}
		}
		if opts == nil {
			opts = &command.PushOptions{}
		}
		opts = ensurePushOptions(opts, j.Repository)
		optsCopy := *opts
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("push:%s:%s", j.Repository.RepoID, optsCopy.RemoteName),
			Timeout:   dynamicTimeout(j.Repository.State.Branch.PushableCount),
			Operation: command.OperationPush,
			Execute: func(ctx context.Context) command.OperationOutcome {
				msg, err := command.PushWithContext(ctx, j.Repository, &optsCopy)
				return command.OperationOutcome{
					Operation:       command.OperationPush,
					Message:         msg,
					Err:             err,
					SuppressSuccess: suppress,
				}
			},
		}
		if err := command.ScheduleGitCommand(j.Repository, req); err != nil {
			return err
		}
	default:
		j.Repository.SetWorkStatus(git.Available)
		return nil
	}
	return nil
}

func ensurePullOptions(opts *command.PullOptions, repo *git.Repository, ffOnly, rebase bool) *command.PullOptions {
	remoteName := "origin"
	if repo.State.Remote != nil && repo.State.Remote.Name != "" {
		remoteName = repo.State.Remote.Name
	}

	if opts == nil {
		opts = &command.PullOptions{}
	}

	if opts.RemoteName == "" {
		opts.RemoteName = remoteName
	}
	if opts.ReferenceName == "" {
		if branch := git.UpstreamBranchName(repo); branch != "" {
			opts.ReferenceName = branch
		}
	}
	if ffOnly {
		opts.FFOnly = true
	}
	if rebase {
		opts.Rebase = true
	}
	return opts
}

func ensurePushOptions(opts *command.PushOptions, repo *git.Repository) *command.PushOptions {
	if opts == nil {
		opts = &command.PushOptions{}
	}
	remoteName := "origin"
	if repo.State.Remote != nil && repo.State.Remote.Name != "" {
		remoteName = repo.State.Remote.Name
	}
	if opts.RemoteName == "" {
		opts.RemoteName = remoteName
	}
	if opts.ReferenceName == "" && repo.State.Branch != nil && repo.State.Branch.Name != "" {
		opts.ReferenceName = repo.State.Branch.Name
	}
	return opts
}

// dynamicTimeout returns a git command timeout scaled by the given change count.
// If the count cannot be determined, the default timeout is returned.
func dynamicTimeout(countFn func() (int, bool)) time.Duration {
	if count, ok := countFn(); ok {
		return command.DynamicTimeout(command.DefaultGitCommandTimeout, count)
	}
	return command.DefaultGitCommandTimeout
}

