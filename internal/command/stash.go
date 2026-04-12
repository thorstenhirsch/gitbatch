package command

import (
	"context"
	"fmt"
	"strings"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// StashOptions defines the rules of the stash push operation.
type StashOptions struct {
	// Message is an optional stash message.
	Message string
}

// StashPopOptions defines the rules of the stash pop operation.
type StashPopOptions struct {
	// StashRef is the stash reference, e.g. "stash@{0}".
	StashRef string
}

// StashDropOptions defines the rules of the stash drop operation.
type StashDropOptions struct {
	// StashRef is the stash reference, e.g. "stash@{0}".
	StashRef string
}

// StashWithContext stashes all local changes with an optional message.
func StashWithContext(ctx context.Context, r *git.Repository, options *StashOptions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"stash", "push"}
	if options != nil && strings.TrimSpace(options.Message) != "" {
		args = append(args, "-m", strings.TrimSpace(options.Message))
	}

	out, err := RunWithContext(ctx, r.AbsPath, "git", args)
	if err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	if strings.Contains(out, "No local changes to save") {
		return "no changes to stash", nil
	}
	return "stash saved", nil
}

// StashPopWithContext pops the specified stash entry.
func StashPopWithContext(ctx context.Context, r *git.Repository, options *StashPopOptions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options == nil || options.StashRef == "" {
		return "", fmt.Errorf("stash reference is required")
	}

	out, err := RunWithContext(ctx, r.AbsPath, "git", []string{"stash", "pop", options.StashRef})
	if err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	return "stash popped", nil
}

// StashDropWithContext drops the specified stash entry.
func StashDropWithContext(ctx context.Context, r *git.Repository, options *StashDropOptions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options == nil || options.StashRef == "" {
		return "", fmt.Errorf("stash reference is required")
	}

	out, err := RunWithContext(ctx, r.AbsPath, "git", []string{"stash", "drop", options.StashRef})
	if err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	return "stash dropped", nil
}
