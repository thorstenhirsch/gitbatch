package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const DefaultFetchTimeout = DefaultGitCommandTimeout

var (
	fetchTryCount int
	fetchMaxTry   = 1
)

// FetchOptions defines the rules for fetch operation
type FetchOptions struct {
	// Name of the remote to fetch from. Defaults to origin.
	RemoteName string
	// Credentials holds the user and password information
	Credentials *git.Credentials
	// Before fetching, remove any remote-tracking references that no longer
	// exist on the remote.
	Prune bool
	// Show what would be done, without making any changes.
	DryRun bool
	// Process logs the output to stdout
	Progress bool
	// Force allows the fetch to update a local branch even when the remote
	// branch does not descend from it.
	Force bool
	// Mode is the command mode
	CommandMode Mode
	// Timeout is the maximum duration allowed for the fetch command when
	// executed via the legacy git CLI. If zero, a sensible default is used.
	Timeout time.Duration
	// There should be more room for authentication, tags and progress
}

// Fetch branches refs from one or more other repositories, along with the
// objects necessary to complete their histories.
func Fetch(r *git.Repository, o *FetchOptions) (message string, err error) {
	return FetchWithContext(context.Background(), r, o)
}

// FetchWithContext performs fetch honouring the supplied context for cancellation and deadlines.
func FetchWithContext(ctx context.Context, r *git.Repository, o *FetchOptions) (message string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// here we configure fetch operation
	// default mode is go-git (this may be configured)
	mode := o.CommandMode
	fetchTryCount = 0
	if o.Timeout <= 0 {
		o.Timeout = DefaultFetchTimeout
	}
	// prune and dry run is not supported from go-git yet, rely on old friend
	if o.Prune || o.DryRun {
		mode = ModeLegacy
	}
	switch mode {
	case ModeLegacy:
		return fetchWithGit(ctx, r, o)
	case ModeNative:
		// this should be the refspec as default, let's give it a try
		// TODO: Fix for quick mode, maybe better read config file
		var refspec string
		if r.State.Branch == nil {
			refspec = "+refs/heads/*:refs/remotes/origin/*"
		} else {
			refspec = "+" + "refs/heads/" + r.State.Branch.Name + ":" + "/refs/remotes/" + r.State.Remote.Name + "/" + r.State.Branch.Name
		}
		return fetchWithGoGit(ctx, r, o, refspec)
	}
	return "", nil
}

// fetchWithGit is simply a bare git fetch <remote> command which is flexible
// for complex operations, but on the other hand, it ties the app to another
// tool. To avoid that, using native implementation is preferred.
func fetchWithGit(ctx context.Context, r *git.Repository, options *FetchOptions) (string, error) {
	args := make([]string, 0)
	args = append(args, "fetch")
	// parse options to command line arguments
	if len(options.RemoteName) > 0 {
		args = append(args, options.RemoteName)
	}
	if options.Prune {
		args = append(args, "-p")
	}
	if options.Force {
		args = append(args, "-f")
	}
	if options.DryRun {
		args = append(args, "--dry-run")
	}
	ref, _ := r.Repo.Head()
	initialRef := shortHash(ref)

	var (
		out    string
		errRun error
	)
	if options.Timeout > 0 {
		out, errRun = RunWithContextTimeout(ctx, r.AbsPath, "git", args, options.Timeout)
	} else {
		out, errRun = RunWithContext(ctx, r.AbsPath, "git", args)
	}
	if errRun != nil {
		if errors.Is(errRun, context.DeadlineExceeded) {
			return "", fmt.Errorf("fetch timed out after %s: %w", options.Timeout, errRun)
		}
		return "", gerr.ParseGitError(out, errRun)
	}
	uRef := "origin/HEAD"
	if r.State.Branch != nil && r.State.Branch.Upstream != nil {
		up := r.State.Branch.Upstream
		switch {
		case up.Reference != nil:
			uRef = shortHash(up.Reference)
		case up.Name != "":
			uRef = up.Name
		}
	}

	msg, err := getFetchMessage(r, initialRef, uRef)
	if err != nil {
		msg = "couldn't get stat"
	}
	return msg, nil
}

// fetchWithGoGit is the primary fetch method and refspec is the main feature.
// RefSpec is a mapping from local branches to remote references The format of
// the refspec is an optional +, followed by <src>:<dst>, where <src> is the
// pattern for references on the remote side and <dst> is where those references
// will be written locally. The + tells Git to update the reference even if it
// isn't a fast-forward.
func fetchWithGoGit(ctx context.Context, r *git.Repository, options *FetchOptions, refspec string) (string, error) {
	opt := &gogit.FetchOptions{
		RemoteName: options.RemoteName,
		RefSpecs:   []config.RefSpec{config.RefSpec(refspec)},
		Force:      options.Force,
	}
	// if any credential is given, let's add it to the git.FetchOptions
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

	// Store initial ref before fetch
	ref, _ := r.Repo.Head()
	initialRef := shortHash(ref)

	if err := r.Repo.Fetch(opt); err != nil {
		if err == gogit.NoErrAlreadyUpToDate {
			uRef := initialRef
			if r.State.Branch != nil && r.State.Branch.Upstream != nil {
				up := r.State.Branch.Upstream
				switch {
				case up.Reference != nil:
					uRef = shortHash(up.Reference)
				case up.Name != "":
					uRef = up.Name
				}
			}
			msg, msgErr := getFetchMessage(r, initialRef, uRef)
			if msgErr != nil {
				msg = "couldn't get stat"
			}
			return msg, nil
		} else if strings.Contains(err.Error(), "couldn't find remote ref") {
			// we don't have remote ref, so lets pull other things.. maybe it'd be useful
			rp := r.State.Remote.RefSpecs[0]
			if fetchTryCount < fetchMaxTry {
				fetchTryCount++
				return fetchWithGoGit(ctx, r, options, rp)
			} else {
				return "", err
			}
		} else if strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
			// The env variable SSH_AUTH_SOCK is not defined, maybe git can handle this
			return fetchWithGit(ctx, r, options)
		} else if err == transport.ErrAuthenticationRequired {
			return "", gerr.ErrAuthenticationRequired
		} else {
			return fetchWithGit(ctx, r, options)
		}
	}

	// Get updated refs after fetching
	uRef := "origin/HEAD"
	if r.State.Branch != nil && r.State.Branch.Upstream != nil {
		up := r.State.Branch.Upstream
		switch {
		case up.Reference != nil:
			uRef = shortHash(up.Reference)
		case up.Name != "":
			uRef = up.Name
		}
	}

	msg, errMsg := getFetchMessage(r, initialRef, uRef)
	if errMsg != nil {
		msg = "couldn't get stat"
	}
	return msg, nil
}

func shortHash(ref *plumbing.Reference) string {
	if ref == nil {
		return ""
	}
	h := ref.Hash().String()
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

func getFetchMessage(r *git.Repository, ref1, ref2 string) (string, error) {
	msg := ref1 + ".." + ref2 + " "
	if ref1 == ref2 {
		msg = msg + "already up-to-date"
	} else {
		out, err := DiffStatRefs(r, ref1, ref2)
		if err != nil {
			return "", err
		}
		re := regexp.MustCompile(`\r?\n`)
		lines := re.Split(out, -1)
		last := lines[len(lines)-1]
		if len(last) > 0 {
			changes := strings.Split(last, ",")
			msg = msg + changes[0][1:]
		}
	}
	return msg, nil
}
