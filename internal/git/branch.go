package git

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Branch is the wrapper of go-git's Reference struct. In addition to that, it
// also holds name of the branch, pullable and pushable commit count from the
// branchs' upstream. It also tracks if the repository has unstaged or uncommit-
// ed changes
type Branch struct {
	Name      string
	Reference *plumbing.Reference
	Upstream  *RemoteBranch
	Commits   []*Commit
	State     *BranchState
	Pushables string
	Pullables string
	Clean     bool
}

// BranchState hold the ref commit
type BranchState struct {
	Commit *Commit
}

const (
	revlistCommand = "rev-list"
	hashLength     = 40
)

// search for branches in go-git way. It is useful to do so that checkout and
// checkout error handling can be handled by code rather than struggling with
// git command and its output
func (r *Repository) initBranches() error {
	lbs := make([]*Branch, 0)
	bs, err := r.Repo.Branches()
	if err != nil {
		return err
	}
	defer bs.Close()
	headRef, err := r.Repo.Head()
	if err != nil {
		return err
	}
	var branchFound bool
	var push, pull string
	_ = bs.ForEach(func(b *plumbing.Reference) error {
		if b.Type() != plumbing.HashReference {
			return nil
		}
		clean := r.isClean()
		branch := &Branch{
			Name:      b.Name().Short(),
			Reference: b,
			State:     &BranchState{},
			Pushables: push,
			Pullables: pull,
			Clean:     clean,
		}
		if b.Name() == headRef.Name() {
			r.State.Branch = branch
			branchFound = true
		}
		lbs = append(lbs, branch)

		return nil
	})
	if !branchFound {
		branch := &Branch{
			Name:      headRef.Hash().String(),
			Reference: headRef,
			State:     &BranchState{},
			Pushables: "?",
			Pullables: "?",
			Clean:     r.isClean(),
		}
		lbs = append(lbs, branch)
		r.State.Branch = branch
	}
	rb, err := getUpstream(r, r.State.Branch.Name)
	if err == nil {
		r.State.Branch.Upstream = rb
	}

	r.Branches = lbs
	return nil
}

// Checkout to given branch. If any errors occur, the method returns it instead
// of returning nil
func (r *Repository) Checkout(b *Branch) error {
	if b.Name == r.State.Branch.Name {
		return nil
	}

	w, err := r.Repo.Worktree()
	if err != nil {
		return err
	}
	if err = w.Checkout(&git.CheckoutOptions{
		Branch: b.Reference.Name(),
	}); err != nil {
		return err
	}
	r.State.Branch = b

	rb, err := getUpstream(r, r.State.Branch.Name)
	if err == nil {
		r.State.Branch.Upstream = rb
	}
	_ = b.initCommits(r)

	if err := r.Publish(BranchUpdated, nil); err != nil {
		return err
	}
	if err := r.SyncRemoteAndBranch(b); err != nil {
		return err
	}
	return r.Publish(RepositoryUpdated, nil)
}

// checking the branch if it has any changes from its head revision. Initially
// I implemented this with go-git but it was incredibly slow and there is also
// an issue about it: https://github.com/src-d/go-git/issues/844
func (r *Repository) isClean() bool {
	args := []string{"status"}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	s := string(out)
	s = strings.TrimSuffix(s, "\n")
	if len(s) >= 0 {
		vs := strings.Split(s, "\n")
		line := vs[len(vs)-1]
		// earlier versions of git returns "working directory clean" instead of
		//"working tree clean" message
		if strings.Contains(line, "working tree clean") ||
			strings.Contains(line, "working directory clean") {
			return true
		}
	}
	return false
}

// RevListOptions defines the rules of rev-list func
type RevListOptions struct {
	// Ref1 is the first reference hash to link
	Ref1 string
	// Ref2 is the second reference hash to link
	Ref2 string
}

// RevListCount returns the count of commits between two references.
// This is more efficient than RevList when you only need the count.
func RevListCount(r *Repository, options RevListOptions) (int, error) {
	// Validate that both references are provided
	if len(options.Ref1) == 0 || len(options.Ref2) == 0 {
		return 0, fmt.Errorf("both Ref1 and Ref2 must be provided")
	}

	args := []string{revlistCommand, "--count"}
	arg1 := options.Ref1 + ".." + options.Ref2
	args = append(args, arg1)

	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("rev-list --count failed: %w (output: %s)", err, string(out))
	}

	s := strings.TrimSpace(string(out))
	if len(s) == 0 {
		return 0, nil
	}

	count, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid count output: %s", s)
	}

	return count, nil
}

// RevList is the legacy implementation of "git rev-list" command.
func RevList(r *Repository, options RevListOptions) ([]*object.Commit, error) {
	args := make([]string, 0)
	args = append(args, revlistCommand)
	if len(options.Ref1) > 0 && len(options.Ref2) > 0 {
		arg1 := options.Ref1 + ".." + options.Ref2
		args = append(args, arg1)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's just an empty result (branches are identical)
		// In this case, git rev-list returns exit code 0 with empty output
		// But some errors like "unknown revision" return exit code 128
		return nil, fmt.Errorf("rev-list failed: %w (output: %s)", err, string(out))
	}
	s := string(out)
	if len(s) == 0 {
		// Empty output means no commits in the range, which is valid
		return make([]*object.Commit, 0), nil
	}
	hashes := strings.Split(s, "\n")
	commits := make([]*object.Commit, 0)
	for _, hash := range hashes {
		if len(hash) == hashLength {
			c, err := r.Repo.CommitObject(plumbing.NewHash(hash))
			if err != nil {
				// Skip invalid commit objects but continue processing
				continue
			}
			commits = append(commits, c)
		}
	}
	sort.Sort(CommitTime(commits))
	return commits, nil
}

// SyncRemoteAndBranch synchronizes remote branch with current branch
func (r *Repository) SyncRemoteAndBranch(b *Branch) error {
	headRef, err := r.Repo.Head()
	if err != nil {
		return err
	}
	if b.Upstream == nil {
		b.Pullables = "?"
		b.Pushables = "?"
		return nil
	}

	// Validate upstream reference exists
	if b.Upstream.Reference == nil {
		b.Pullables = "?"
		b.Pushables = "?"
		return nil
	}

	head := headRef.Hash().String()
	upstreamHash := b.Upstream.Reference.Hash().String()

	// Validate that both hashes are valid (40 character hex strings)
	if len(head) != hashLength || len(upstreamHash) != hashLength {
		b.Pullables = "?"
		b.Pushables = "?"
		return nil
	}

	var push, pull string

	// Calculate pushables (commits in local that are not in upstream)
	// Use RevListCount for better performance and resilience
	pushCount, err := RevListCount(r, RevListOptions{
		Ref1: upstreamHash,
		Ref2: head,
	})
	if err != nil {
		// On error, keep trying for pullables instead of failing completely
		push = "?"
	} else {
		push = strconv.Itoa(pushCount)
	}

	// Calculate pullables (commits in upstream that are not in local)
	pullCount, err := RevListCount(r, RevListOptions{
		Ref1: head,
		Ref2: upstreamHash,
	})
	if err != nil {
		// On error, set to unknown but don't fail the whole operation
		pull = "?"
	} else {
		pull = strconv.Itoa(pullCount)
	}

	b.Pullables = pull
	b.Pushables = push
	return nil
}

// InitializeCommits loads the commits
func (b *Branch) InitializeCommits(r *Repository) error {
	return b.initCommits(r)
}

func getUpstream(r *Repository, branchName string) (*RemoteBranch, error) {
	args := []string{"config", "--get", "branch." + branchName + ".remote"}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	cr, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("upstream not found: %w", err)
	}

	remoteName := strings.TrimSpace(string(cr))
	if remoteName == "" {
		return nil, fmt.Errorf("upstream remote is empty")
	}

	args = []string{"config", "--get", "branch." + branchName + ".merge"}
	cmd = exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	cm, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("upstream merge config not found: %w", err)
	}

	mergeRef := strings.TrimSpace(string(cm))
	if mergeRef == "" || !strings.Contains(mergeRef, branchName) {
		return nil, fmt.Errorf("invalid merge branch configuration")
	}

	// Find the remote by name
	var targetRemote *Remote
	for _, rm := range r.Remotes {
		if rm.Name == remoteName {
			targetRemote = rm
			r.State.Remote = rm
			break
		}
	}

	if targetRemote == nil {
		return nil, fmt.Errorf("remote %s not found in repository", remoteName)
	}

	// Find the remote branch
	targetBranchName := targetRemote.Name + "/" + branchName
	for _, rb := range targetRemote.Branches {
		if rb.Name == targetBranchName {
			return rb, nil
		}
	}

	return nil, fmt.Errorf("upstream branch %s not found", targetBranchName)
}
