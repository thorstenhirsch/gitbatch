package command

import (
	"context"
	"strings"

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
			if err := git.AcquireGitSemaphore(context.Background()); err != nil {
				return
			}
			defer git.ReleaseGitSemaphore()

			err := r.Refresh()
			if err != nil {
				ScheduleStateEvaluation(r, OperationOutcome{
					Operation: OperationRefresh,
					Err:       err,
				})
				return
			}

			if outcome == nil {
				// Auto-refresh triggered after a successful pull/merge/push/fetch.
				// The operation already updated remote-tracking refs, so only a
				// cleanliness re-check is needed — not another ls-remote + git fetch
				// (which handleStateProbe would schedule). Routing through
				// OperationStateProbe completion calls applyCleanliness without
				// triggering a new remote probe, eliminating the 2–3 s delay.
				message := " " // non-empty: tells EvaluateRepositoryState this is a completion, not an initial probe
				if r.State != nil && strings.TrimSpace(r.State.Message) != "" {
					message = r.State.Message
				}
				ScheduleStateEvaluation(r, OperationOutcome{
					Operation: OperationStateProbe,
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
