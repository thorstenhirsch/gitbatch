package command

import (
	"fmt"
	"strings"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// ReevaluateRepositoryState recomputes derived repository state after operations that
// may have modified the working tree outside gitbatch (e.g. fetch, lazygit).
func ReevaluateRepositoryState(r *git.Repository) {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return
	}

	branch := r.State.Branch

	if branch.HasIncomingCommits() {
		upstream := branch.Upstream
		if upstream == nil || upstream.Name == "" {
			return
		}
		if fastForwardDryRunSucceeds(r, upstream) {
			r.MarkClean()
		}
		return
	}

	upstream := branch.Upstream
	if upstream == nil {
		r.MarkRecoverableError("upstream not configured")
		return
	}

	remoteName, remoteBranch := resolveUpstreamParts(r, branch)
	if remoteName == "" || remoteBranch == "" {
		r.MarkRecoverableError("upstream not configured")
		return
	}

	exists, err := upstreamExistsOnRemote(r, remoteName, remoteBranch)
	if err != nil {
		r.MarkRecoverableError(fmt.Sprintf("unable to verify upstream: %v", err))
		return
	}
	if !exists {
		r.MarkRecoverableError(fmt.Sprintf("upstream %s missing on remote", remoteName+"/"+remoteBranch))
		return
	}

	r.MarkClean()
}

func resolveUpstreamParts(r *git.Repository, branch *git.Branch) (string, string) {
	if branch == nil || branch.Upstream == nil {
		return "", ""
	}

	var remoteName string
	remoteBranch := branch.Name

	if branch.Upstream.Reference != nil {
		short := branch.Upstream.Reference.Name().Short()
		if parts := strings.SplitN(short, "/", 2); len(parts) == 2 {
			remoteName = parts[0]
			remoteBranch = parts[1]
		}
	} else if branch.Upstream.Name != "" {
		short := branch.Upstream.Name
		short = strings.TrimPrefix(short, "refs/remotes/")
		if parts := strings.SplitN(short, "/", 2); len(parts) == 2 {
			remoteName = parts[0]
			remoteBranch = parts[1]
		}
	}

	if strings.EqualFold(remoteBranch, "HEAD") {
		remoteBranch = branch.Name
	}
	if remoteName == "" && r.State != nil && r.State.Remote != nil {
		remoteName = r.State.Remote.Name
	}

	return remoteName, remoteBranch
}

func upstreamExistsOnRemote(r *git.Repository, remoteName, branchName string) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("repository not initialized")
	}
	if remoteName == "" || branchName == "" {
		return false, fmt.Errorf("remote or branch missing")
	}

	branchRef := branchName
	if !strings.HasPrefix(branchRef, "refs/") {
		branchRef = "refs/heads/" + branchRef
	}

	args := []string{"ls-remote", "--heads", remoteName, branchRef}
	out, err := RunWithTimeout(r.AbsPath, "git", args, DefaultFetchTimeout)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func fastForwardDryRunSucceeds(r *git.Repository, upstream *git.RemoteBranch) bool {
	if r == nil || upstream == nil {
		return false
	}

	upstreamRef := upstream.Name
	if upstreamRef == "" && upstream.Reference != nil {
		upstreamRef = upstream.Reference.Name().String()
	}
	if upstreamRef == "" {
		return false
	}

	headHash, err := Run(r.AbsPath, "git", []string{"rev-parse", "HEAD"})
	if err != nil {
		return false
	}

	mergeBase, err := Run(r.AbsPath, "git", []string{"merge-base", "HEAD", upstreamRef})
	if err != nil {
		return false
	}

	return strings.TrimSpace(headHash) != "" && strings.TrimSpace(headHash) == strings.TrimSpace(mergeBase)
}
