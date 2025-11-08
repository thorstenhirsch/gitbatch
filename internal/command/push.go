package command

import (
	"fmt"
	"strings"

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
	// Mode is the command mode
	CommandMode Mode
}

func Push(r *git.Repository, options *PushOptions) error {
	if options == nil {
		options = &PushOptions{}
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
	out, err := Run(r.AbsPath, "git", args)
	if err != nil {
		trimmed := strings.TrimSpace(out)
		base := gerr.ParseGitError(out, err)
		if trimmed == "" {
			return base
		}
		if base == gerr.ErrUnclassified {
			return fmt.Errorf("%s", trimmed)
		}
		return fmt.Errorf("%w: %s", base, trimmed)
	}
	r.SetWorkStatus(git.Success)
	r.State.Message = "push completed"
	return r.Refresh()
}
