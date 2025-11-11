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

		// Run refresh asynchronously to avoid blocking the state queue
		// This is critical during initial load when many repos refresh simultaneously
		go func() {
			err := r.Refresh()
			if err != nil {
				ScheduleStateEvaluation(r, OperationOutcome{
					Operation: OperationRefresh,
					Err:       err,
				})
				return
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
				return
			}

			outcomeToSend := *outcome
			if outcomeToSend.Operation == "" {
				outcomeToSend.Operation = OperationRefresh
			}
			ScheduleStateEvaluation(r, outcomeToSend)
		}()
		return nil
	})
}

func init() {
	git.RegisterRepositoryHook(AttachRefreshExecutor)
}
