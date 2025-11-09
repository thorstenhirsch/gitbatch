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

// refreshBranchState reloads repository metadata and ensures branch commits are initialized.
func refreshBranchState(repo *git.Repository) error {
	if repo == nil {
		return nil
	}
	if err := repo.ForceRefresh(); err != nil {
		command.ScheduleStateEvaluation(repo, command.OperationOutcome{
			Operation: command.OperationRefresh,
			Err:       err,
			Message:   err.Error(),
		})
		return err
	}
	if state := repo.State; state != nil && state.Branch != nil {
		if err := state.Branch.InitializeCommits(repo); err != nil {
			command.ScheduleStateEvaluation(repo, command.OperationOutcome{
				Operation: command.OperationRefresh,
				Err:       err,
				Message:   err.Error(),
			})
			return err
		}
	}
	message := ""
	if repo.State != nil {
		message = repo.State.Message
	}
	command.ScheduleStateEvaluation(repo, command.OperationOutcome{
		Operation: command.OperationRefresh,
		Message:   message,
	})
	return nil
}
