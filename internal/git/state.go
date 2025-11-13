package git

import (
	"strings"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
)

// MarkDisabled marks the current branch as having unmerged or unpushed work.
// This reflects the repository entering a disabled state until reconciled or refreshed.
func (r *Repository) MarkDisabled() {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return
	}

	r.State.Branch.Clean = false
	for _, candidate := range r.Branches {
		if candidate != nil && candidate.Name == r.State.Branch.Name {
			candidate.Clean = false
		}
	}
}

// MarkClean updates the current branch as clean.
func (r *Repository) MarkClean() {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return
	}

	r.State.Branch.Clean = true
	for _, candidate := range r.Branches {
		if candidate != nil && candidate.Name == r.State.Branch.Name {
			candidate.Clean = true
		}
	}
}

// MarkCriticalError transitions the repository into a critical error state.
func (r *Repository) MarkCriticalError(message string) {
	r.markErrorState(message, false)
}

// MarkRecoverableError transitions the repository into a recoverable error state.
func (r *Repository) MarkRecoverableError(message string) {
	r.markErrorState(message, true)
}

// MarkRequiresCredentials transitions the repository into a state requiring credentials.
func (r *Repository) MarkRequiresCredentials(message string) {
	if r == nil {
		return
	}

	r.MarkDisabled()
	r.SetWorkStatus(Fail)
	if r.State == nil {
		return
	}
	r.State.RequiresCredentials = true
	r.State.RecoverableError = false
	trimmed := strings.TrimSpace(message)
	if trimmed != "" {
		r.State.Message = trimmed
	} else {
		r.State.Message = "authentication required"
	}
}

func (r *Repository) markErrorState(message string, recoverable bool) {
	if r == nil {
		return
	}

	r.MarkDisabled()
	r.SetWorkStatus(Fail)
	if r.State == nil {
		return
	}
	r.State.RecoverableError = recoverable
	r.State.RequiresCredentials = false
	trimmed := strings.TrimSpace(message)
	if trimmed != "" {
		r.State.Message = trimmed
	}
}

// ApplyOperationError normalises an error from a git command and assigns
// the corresponding repository state.
func (r *Repository) ApplyOperationError(err error) error {
	if err == nil {
		return nil
	}
	if r == nil {
		return err
	}

	message := NormalizeGitErrorMessage(err.Error())
	if gerr.RequiresCredentials(err) {
		r.MarkRequiresCredentials(message)
	} else if gerr.IsRecoverable(err) {
		r.MarkRecoverableError(message)
	} else {
		r.MarkCriticalError(message)
	}
	return err
}

// NormalizeGitErrorMessage trims git error noise so it can be shown in the UI.
func NormalizeGitErrorMessage(msg string) string {
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.TrimSpace(msg)
	msg = strings.TrimPrefix(msg, gerr.ErrUnclassified.Error()+": ")
	if msg == gerr.ErrUnclassified.Error() {
		msg = ""
	}
	if msg == "" {
		return "unknown error"
	}
	fields := strings.Fields(msg)
	if len(fields) == 0 {
		return "unknown error"
	}
	return strings.Join(fields, " ")
}
