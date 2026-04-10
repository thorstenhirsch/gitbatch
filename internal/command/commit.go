package command

import (
	"context"
	"fmt"
	"strings"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// CommitOptions defines the rules of the commit operation
type CommitOptions struct {
	// Message is the commit summary (first line).
	Message string
	// Description is the optional extended commit body.
	Description string
}

// CommitWithContext stages all changes and commits with the given message.
func CommitWithContext(ctx context.Context, r *git.Repository, options *CommitOptions) (string, error) {
	if options == nil || strings.TrimSpace(options.Message) == "" {
		return "", fmt.Errorf("commit message is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Stage all changes
	out, err := RunWithContext(ctx, r.AbsPath, "git", []string{"add", "-A"})
	if err != nil {
		return "", gerr.ParseGitError(out, err)
	}

	// Build commit message
	msg := strings.TrimSpace(options.Message)
	if desc := strings.TrimSpace(options.Description); desc != "" {
		msg = msg + "\n\n" + desc
	}

	// Commit
	out, err = RunWithContext(ctx, r.AbsPath, "git", []string{"commit", "-m", msg})
	if err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	return "commit completed", nil
}
