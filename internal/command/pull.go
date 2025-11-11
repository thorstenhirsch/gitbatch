package command

import (
	"context"

	"github.com/go-git/go-git/v5/plumbing"
	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

var (
	pullTryCount int
)

// PullOptions defines the rules for pull operation
type PullOptions struct {
	// Name of the remote to fetch from. Defaults to origin.
	RemoteName string
	// ReferenceName Remote branch to clone. If empty, uses HEAD.
	ReferenceName string
	// Fetch only ReferenceName if true.
	SingleBranch bool
	// Credentials holds the user and password information
	Credentials *git.Credentials
	// Process logs the output to stdout
	Progress bool
	// Force allows the pull to update a local branch even when the remote
	// branch does not descend from it.
	Force bool
	// FFOnly ensures only fast-forward merges are allowed.
	FFOnly bool
	// Rebase performs the pull using rebase instead of merge.
	Rebase bool
}

// Pull incorporates changes from a remote repository into the current branch.
func Pull(r *git.Repository, o *PullOptions) (string, error) {
	return PullWithContext(context.Background(), r, o)
}

// PullWithContext performs pull respecting context cancellation and deadlines.
func PullWithContext(ctx context.Context, r *git.Repository, o *PullOptions) (string, error) {
	pullTryCount = 0

	if o == nil {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return pullWithGit(ctx, r, o)
}

func pullWithGit(ctx context.Context, r *git.Repository, options *PullOptions) (string, error) {
	args := make([]string, 0)
	args = append(args, "pull")
	// parse options to command line arguments
	if options.FFOnly {
		args = append(args, "--ff-only")
	}
	if options.Rebase {
		args = append(args, "--rebase")
	}
	if options.Force {
		args = append(args, "-f")
	}
	if len(options.RemoteName) > 0 {
		args = append(args, options.RemoteName)
	}
	if len(options.ReferenceName) > 0 {
		args = append(args, options.ReferenceName)
	}
	ref, _ := r.Repo.Head()
	if out, err := RunWithContext(ctx, r.AbsPath, "git", args); err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	newref, _ := r.Repo.Head()

	msg, err := getMergeMessage(r, referenceHash(ref), referenceHash(newref))
	if err != nil {
		msg = "couldn't get stat"
	}
	return msg, nil
}

func referenceHash(ref *plumbing.Reference) string {
	if ref == nil {
		return ""
	}
	return ref.Hash().String()
}
