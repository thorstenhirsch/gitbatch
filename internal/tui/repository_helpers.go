package tui

import (
	"slices"

	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// filterRepositories returns a copy of the provided slice without nil repositories.
func filterRepositories(repos []*git.Repository) []*git.Repository {
	if len(repos) == 0 {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(repos), func(repo *git.Repository) bool {
		return repo == nil
	})
}

// scheduleRefresh enqueues a repository refresh and propagates any scheduling errors.
func scheduleRefresh(repo *git.Repository) error {
	if repo == nil {
		return nil
	}
	message := ""
	if repo.State != nil {
		message = repo.State.Message
	}
	if err := command.ScheduleRepositoryRefresh(repo, &command.OperationOutcome{
		Operation: command.OperationRefresh,
		Message:   message,
	}); err != nil {
		command.ScheduleStateEvaluation(repo, command.OperationOutcome{
			Operation: command.OperationRefresh,
			Err:       err,
			Message:   err.Error(),
		})
		return err
	}
	return nil
}
