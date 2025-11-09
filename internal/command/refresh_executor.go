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
		var outcome *OperationOutcome
		switch payload := event.Data.(type) {
		case OperationOutcome:
			tmp := payload
			outcome = &tmp
		case *OperationOutcome:
			if payload != nil {
				tmp := *payload
				outcome = &tmp
			}
		case nil:
			outcome = nil
		}

		err := r.Refresh()
		if err != nil {
			ScheduleStateEvaluation(r, OperationOutcome{
				Operation: OperationRefresh,
				Err:       err,
			})
			return err
		}

		if outcome == nil {
			message := ""
			if r.State != nil {
				message = r.State.Message
			}
			ScheduleStateEvaluation(r, OperationOutcome{
				Operation: OperationRefresh,
				Message:   message,
			})
			return nil
		}

		if outcome.Operation == "" {
			outcome.Operation = OperationRefresh
		}
		ScheduleStateEvaluation(r, *outcome)
		return nil
	})
}

func init() {
	git.RegisterRepositoryHook(AttachRefreshExecutor)
}
