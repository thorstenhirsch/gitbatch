package command

import (
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// AttachRefreshExecutor registers the listener responsible for executing refresh
// requests on the TUI queue.
func AttachRefreshExecutor(r *git.Repository) {
	if r == nil {
		return
	}
	r.On(git.RepositoryRefreshRequested, func(event *git.RepositoryEvent) error {
		err := r.Refresh()
		if err != nil {
			ScheduleStateEvaluation(r, OperationOutcome{
				Operation: OperationRefresh,
				Err:       err,
			})
		}
		return err
	})
}

func init() {
	git.RegisterRepositoryHook(AttachRefreshExecutor)
}
