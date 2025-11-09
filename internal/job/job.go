package job

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	j.Repository.SetWorkStatus(git.Working)
	// TODO: Better implementation required
	switch mode := j.JobType; mode {
	case FetchJob:
		j.Repository.State.Message = "fetching.."
		var opts *command.FetchOptions
		if j.Options != nil {
			opts = j.Options.(*command.FetchOptions)
		} else {
			remoteName := "origin"
			if j.Repository.State.Remote != nil && j.Repository.State.Remote.Name != "" {
				remoteName = j.Repository.State.Remote.Name
			}
			opts = &command.FetchOptions{
				RemoteName:  remoteName,
				CommandMode: command.ModeLegacy,
				Timeout:     command.DefaultFetchTimeout,
			}
		}
		if branch := j.Repository.State.Branch; branch != nil && branch.Upstream != nil {
			if branch.Upstream.Reference == nil {
				recoverable := true
				upstreamName := strings.TrimSpace(branch.Upstream.Name)
				msg := "upstream missing on remote"
				if upstreamName != "" {
					msg = fmt.Sprintf("upstream %s missing on remote", upstreamName)
				}
				command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
					Operation:           command.OperationFetch,
					Err:                 errors.New(msg),
					Message:             msg,
					RecoverableOverride: &recoverable,
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
		j.Repository.State.Message = "pulling.."
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
			recoverable := true
			msg := "upstream not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationPull,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
			})
			return nil
		}
		if j.Repository.State.Remote == nil {
			recoverable := false
			msg := "remote not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationPull,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
			})
			return nil
		}
		opts = ensurePullOptions(opts, j.Repository, true, false)
		optsCopy := *opts
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("pull:%s:%s", j.Repository.RepoID, optsCopy.RemoteName),
			Timeout:   command.DefaultGitCommandTimeout,
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
		j.Repository.State.Message = "merging.."
		if j.Repository.State == nil || j.Repository.State.Branch == nil || j.Repository.State.Branch.Upstream == nil {
			recoverable := true
			msg := "upstream not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationMerge,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
			})
			return nil
		}
		optsCopy := command.MergeOptions{BranchName: j.Repository.State.Branch.Upstream.Name}
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("merge:%s:%s", j.Repository.RepoID, optsCopy.BranchName),
			Timeout:   command.DefaultGitCommandTimeout,
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
		j.Repository.State.Message = "rebasing.."
		if j.Repository.State == nil || j.Repository.State.Branch == nil || j.Repository.State.Branch.Upstream == nil {
			recoverable := true
			msg := "upstream not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationRebase,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
			})
			return nil
		}
		if j.Repository.State.Remote == nil {
			recoverable := false
			msg := "remote not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationRebase,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
			})
			return nil
		}
		var opts *command.PullOptions
		if j.Options != nil {
			opts = j.Options.(*command.PullOptions)
		} else {
			opts = &command.PullOptions{}
		}
		opts = ensurePullOptions(opts, j.Repository, false, true)
		optsCopy := *opts
		req := &command.GitCommandRequest{
			Key:       fmt.Sprintf("rebase:%s:%s", j.Repository.RepoID, optsCopy.RemoteName),
			Timeout:   command.DefaultGitCommandTimeout,
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
		j.Repository.State.Message = "pushing.."
		if j.Repository.State == nil || j.Repository.State.Remote == nil {
			recoverable := false
			msg := "remote not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationPush,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
			})
			return nil
		}
		if j.Repository.State.Branch == nil {
			recoverable := false
			msg := "branch not set"
			command.ScheduleStateEvaluation(j.Repository, command.OperationOutcome{
				Operation:           command.OperationPush,
				Err:                 errors.New(msg),
				Message:             msg,
				RecoverableOverride: &recoverable,
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
			Timeout:   command.DefaultGitCommandTimeout,
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
	// Force CLI execution to respect ff-only/rebase options
	if opts.CommandMode == command.ModeNative {
		opts.CommandMode = command.ModeLegacy
	}
	if opts.CommandMode != command.ModeLegacy && opts.CommandMode != command.ModeNative {
		opts.CommandMode = command.ModeLegacy
	}
	if opts.ReferenceName == "" {
		if branch := branchNameForPull(repo); branch != "" {
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
	if opts.CommandMode != command.ModeLegacy && opts.CommandMode != command.ModeNative {
		opts.CommandMode = command.ModeLegacy
	}
	return opts
}

func branchNameForPull(repo *git.Repository) string {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return ""
	}
	if repo.State.Branch.Upstream != nil && repo.State.Branch.Upstream.Name != "" {
		parts := strings.SplitN(repo.State.Branch.Upstream.Name, "/", 2)
		if len(parts) == 2 && parts[1] != "" {
			return parts[1]
		}
	}
	return repo.State.Branch.Name
}
