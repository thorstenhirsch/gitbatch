package job

import (
	"fmt"

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
	Options any
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

	// CommitJob is wrapper of git add -A && git commit
	CommitJob Type = "commit"

	// StashJob is wrapper of git stash push
	StashJob Type = "stash"

	// StashPopJob is wrapper of git stash pop
	StashPopJob Type = "stash-pop"

	// StashDropJob is wrapper of git stash drop
	StashDropJob Type = "stash-drop"
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
	starter, ok := jobStarters[j.JobType]
	if !ok {
		j.Repository.SetWorkStatus(git.Available)
		return nil
	}

	return starter(j)
}
