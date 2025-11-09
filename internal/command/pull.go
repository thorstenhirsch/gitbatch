package command

import (
	"os"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage"
	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

var (
	pullTryCount int
	pullMaxTry   = 1
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
	// Mode is the command mode
	CommandMode Mode
}

// Pull incorporates changes from a remote repository into the current branch.
func Pull(r *git.Repository, o *PullOptions) (string, error) {
	pullTryCount = 0

	if o == nil {
		return "", nil
	}

	// here we configure pull operation
	if o.CommandMode == ModeNative && (o.FFOnly || o.Rebase) {
		return pullWithGit(r, o)
	}

	switch o.CommandMode {
	case ModeLegacy:
		return pullWithGit(r, o)
	case ModeNative:
		return pullWithGoGit(r, o)
	}
	return "", nil
}

func pullWithGit(r *git.Repository, options *PullOptions) (string, error) {
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
	if out, err := Run(r.AbsPath, "git", args); err != nil {
		return "", gerr.ParseGitError(out, err)
	}
	newref, _ := r.Repo.Head()
	if err := r.ForceRefresh(); err != nil {
		return "", err
	}

	msg, err := getMergeMessage(r, referenceHash(ref), referenceHash(newref))
	if err != nil {
		msg = "couldn't get stat"
	}
	return msg, nil
}

func pullWithGoGit(r *git.Repository, options *PullOptions) (string, error) {
	if options.FFOnly || options.Rebase {
		return pullWithGit(r, options)
	}

	opt := &gogit.PullOptions{
		RemoteName:   options.RemoteName,
		SingleBranch: options.SingleBranch,
		Force:        options.Force,
	}
	if len(options.ReferenceName) > 0 {
		ref := plumbing.NewRemoteReferenceName(options.RemoteName, options.ReferenceName)
		opt.ReferenceName = ref
	}
	// if any credential is given, let's add it to the git.PullOptions
	if options.Credentials != nil {
		protocol, err := git.AuthProtocol(r.State.Remote)
		if err != nil {
			return "", err
		}
		if protocol == git.AuthProtocolHTTP || protocol == git.AuthProtocolHTTPS {
			opt.Auth = &http.BasicAuth{
				Username: options.Credentials.User,
				Password: options.Credentials.Password,
			}
		} else {
			return "", gerr.ErrInvalidAuthMethod
		}
	}
	if options.Progress {
		opt.Progress = os.Stdout
	}
	w, err := r.Repo.Worktree()
	if err != nil {
		return "", err
	}
	ref, _ := r.Repo.Head()
	if err = w.Pull(opt); err != nil {
		if err == gogit.NoErrAlreadyUpToDate {
			// log.Error("error: " + err.Error())
			// Already up-to-date
			// TODO: submit a PR for this kind of error, this type of catch is lame
			if errRefresh := r.ForceRefresh(); errRefresh != nil {
				return "", errRefresh
			}
			msg, msgErr := getMergeMessage(r, referenceHash(ref), referenceHash(ref))
			if msgErr != nil {
				msg = "couldn't get stat"
			}
			return msg, nil
		} else if err == storage.ErrReferenceHasChanged && pullTryCount < pullMaxTry {
			pullTryCount++
			if _, err := Fetch(r, &FetchOptions{
				RemoteName: options.RemoteName,
			}); err != nil {
				return "", err
			}
			return Pull(r, options)
		} else if strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
			// The env variable SSH_AUTH_SOCK is not defined, maybe git can handle this
			return pullWithGit(r, options)
		} else if err == transport.ErrAuthenticationRequired {
			return "", gerr.ErrAuthenticationRequired
		} else {
			return pullWithGit(r, options)
		}
	}
	newref, _ := r.Repo.Head()
	if err := r.ForceRefresh(); err != nil {
		return "", err
	}

	msg, errMsg := getMergeMessage(r, referenceHash(ref), referenceHash(newref))
	if errMsg != nil {
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
