package command

import (
	"context"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// PushOptions defines the rules of the push operation
type PushOptions struct {
	// Name of the remote to push to. Defaults to origin.
	RemoteName string
	// ReferenceName identifies the ref to push. Defaults to the current branch name.
	ReferenceName string
	// Force toggles --force pushes when required.
	Force bool
}

func Push(r *git.Repository, options *PushOptions) (string, error) {
	return PushWithContext(context.Background(), r, options)
}

// PushWithContext runs git push and respects context cancellation and deadlines.
func PushWithContext(ctx context.Context, r *git.Repository, options *PushOptions) (string, error) {
	if options == nil {
		options = &PushOptions{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	remote := options.RemoteName
	if remote == "" {
		remote = "origin"
		if r.State.Remote != nil && r.State.Remote.Name != "" {
			remote = r.State.Remote.Name
		}
	}
	ref := options.ReferenceName
	if ref == "" && r.State.Branch != nil {
		ref = r.State.Branch.Name
	}

	args := []string{"push"}
	if options.Force {
		args = append(args, "--force")
	}
	if remote != "" {
		args = append(args, remote)
	}
	if ref != "" {
		args = append(args, ref)
	}
	out, err := RunWithContext(ctx, r.AbsPath, "git", args)
	if err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	return "push completed", nil
}
