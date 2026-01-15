package command

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const DefaultFetchTimeout = 60 * time.Second

var (
	fetchTryCount int
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
	fetchTryCount = 0
	if o.Timeout <= 0 {
		o.Timeout = DefaultFetchTimeout
	}
	return fetchWithGit(ctx, r, o)
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
