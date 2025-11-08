package job

import (
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

// starts the job
func (j *Job) start() error {
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
		if err := command.Fetch(j.Repository, opts); err != nil {
			return j.Repository.ApplyOperationError(err)
		}
		if j.Repository.State != nil && j.Repository.State.RecoverableError {
			// Preserve recoverable failure state surfaced during fetch.
			if j.Repository.WorkStatus() != git.Fail {
				j.Repository.SetWorkStatus(git.Fail)
			}
		} else {
			j.Repository.SetWorkStatus(git.Available)
			if j.Repository.State != nil {
				j.Repository.State.Message = ""
			}
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
		if j.Repository.State.Branch.Upstream == nil {
			j.Repository.MarkRecoverableError("upstream not set")
			return nil
		}
		if j.Repository.State.Remote == nil {
			j.Repository.MarkCriticalError("remote not set")
			return nil
		}
		opts = ensurePullOptions(opts, j.Repository, true, false)
		if err := command.Pull(j.Repository, opts); err != nil {
			return j.Repository.ApplyOperationError(err)
		}
		if suppress {
			j.Repository.SetWorkStatus(git.Available)
		} else {
			j.Repository.SetWorkStatus(git.Success)
		}
		j.Repository.State.Message = "pull completed"
	case MergeJob:
		j.Repository.State.Message = "merging.."
		if j.Repository.State.Branch.Upstream == nil {
			j.Repository.MarkRecoverableError("upstream not set")
			return nil
		}
		if err := command.Merge(j.Repository, &command.MergeOptions{
			BranchName: j.Repository.State.Branch.Upstream.Name,
		}); err != nil {
			return j.Repository.ApplyOperationError(err)
		}
		j.Repository.SetWorkStatus(git.Success)
		j.Repository.State.Message = "merge completed"
	case RebaseJob:
		j.Repository.State.Message = "rebasing.."
		if j.Repository.State.Branch.Upstream == nil {
			j.Repository.MarkRecoverableError("upstream not set")
			return nil
		}
		if j.Repository.State.Remote == nil {
			j.Repository.MarkCriticalError("remote not set")
			return nil
		}
		var opts *command.PullOptions
		if j.Options != nil {
			opts = j.Options.(*command.PullOptions)
		} else {
			opts = &command.PullOptions{}
		}
		opts = ensurePullOptions(opts, j.Repository, false, true)
		if err := command.Pull(j.Repository, opts); err != nil {
			return j.Repository.ApplyOperationError(err)
		}
		j.Repository.SetWorkStatus(git.Success)
		j.Repository.State.Message = "rebase completed"
	case PushJob:
		j.Repository.State.Message = "pushing.."
		if j.Repository.State.Remote == nil {
			j.Repository.MarkCriticalError("remote not set")
			return nil
		}
		if j.Repository.State.Branch == nil {
			j.Repository.MarkCriticalError("branch not set")
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
		if err := command.Push(j.Repository, opts); err != nil {
			return j.Repository.ApplyOperationError(err)
		}
		if suppress {
			j.Repository.SetWorkStatus(git.Available)
		} else {
			j.Repository.SetWorkStatus(git.Success)
		}
		j.Repository.State.Message = "push completed"
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
