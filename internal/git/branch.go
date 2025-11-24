package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var trackRegex = regexp.MustCompile(`\[(?:ahead (\d+))?(?:, )?(?:behind (\d+))?\]`)

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

	// Check cleanliness once for the repository
	status, err := r.GetWorkTreeStatus()
	isRepoClean := err == nil && status.Clean

	// Use git for-each-ref to get all branch info in one go
	args := []string{
		"for-each-ref",
		"--format=%(HEAD)|%(refname)|%(objectname)|%(upstream:short)|%(upstream:track)",
		"refs/heads",
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		isHead := parts[0] == "*"
		refName := parts[1]
		hash := parts[2]
		upstreamShort := parts[3]
		track := parts[4]

		name := strings.TrimPrefix(refName, "refs/heads/")

		var push, pull string
		if track != "" {
			matches := trackRegex.FindStringSubmatch(track)
			if len(matches) > 0 {
				if matches[1] != "" {
					push = matches[1]
				} else {
					push = "0"
				}
				if matches[2] != "" {
					pull = matches[2]
				} else {
					pull = "0"
				}
			} else if track == "[gone]" {
				push = "?"
				pull = "?"
			}
		} else {
			if upstreamShort != "" {
				push = "0"
				pull = "0"
			} else {
				push = "?"
				pull = "?"
			}
		}

		ref := plumbing.NewHashReference(plumbing.ReferenceName(refName), plumbing.NewHash(hash))

		clean := true
		if isHead {
			clean = isRepoClean
		}

		branch := &Branch{
			Name:      name,
			Reference: ref,
			State:     &BranchState{},
			Pushables: push,
			Pullables: pull,
			Clean:     clean,
		}

		if upstreamShort != "" {
			for _, remote := range r.Remotes {
				for _, rb := range remote.Branches {
					if rb.Name == upstreamShort {
						branch.Upstream = rb
						break
					}
				}
				if branch.Upstream != nil {
					break
				}
			}
		}

		lbs = append(lbs, branch)
		if isHead {
			r.State.Branch = branch
		}
	}

	if r.State.Branch == nil {
		headRef, err := r.Repo.Head()
		if err == nil {
			branch := &Branch{
				Name:      headRef.Hash().String(),
				Reference: headRef,
				State:     &BranchState{},
				Pushables: "?",
				Pullables: "?",
				Clean:     isRepoClean,
			}
			lbs = append(lbs, branch)
			r.State.Branch = branch
		}
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

	_ = b.initCommits(r)

	if err := r.Publish(BranchUpdated, nil); err != nil {
		return err
	}
	return nil
}

// RevListOptions defines the rules of rev-list func
type RevListOptions struct {
	// Ref1 is the first reference hash to link
	Ref1 string
	// Ref2 is the second reference hash to link
	Ref2 string
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

// InitializeCommits loads the commits
func (b *Branch) InitializeCommits(r *Repository) error {
	return b.initCommits(r)
}

// PullableCount returns the number of commits available to pull along with an indicator
// specifying whether the count could be determined.
func (b *Branch) PullableCount() (int, bool) {
	if b == nil || b.Upstream == nil {
		return 0, false
	}
	value := strings.TrimSpace(b.Pullables)
	if value == "" || value == "?" {
		return 0, false
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return count, true
}

// HasIncomingCommits reports whether the branch has pullable commits from its upstream.
func (b *Branch) HasIncomingCommits() bool {
	count, ok := b.PullableCount()
	if !ok {
		return false
	}
	return count > 0
}

// WorkTreeStatus represents the status of the working tree
type WorkTreeStatus struct {
	Clean        bool
	HasConflicts bool
}

// GetWorkTreeStatus checks the working tree status using git status --porcelain.
// It returns whether the tree is clean and if there are any conflicts.
func (r *Repository) GetWorkTreeStatus() (WorkTreeStatus, error) {
	args := []string{"status", "--porcelain"}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return WorkTreeStatus{}, err
	}

	s := string(out)
	if len(strings.TrimSpace(s)) == 0 {
		return WorkTreeStatus{Clean: true, HasConflicts: false}, nil
	}

	hasConflicts := false
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		// XY format.
		// U = Unmerged
		// DD = both deleted
		// AU = added by us
		// UD = deleted by them
		// UA = added by them
		// DU = deleted by us
		// AA = both added
		// UU = both modified
		status := line[:2]
		if strings.Contains(status, "U") || status == "DD" || status == "AA" {
			hasConflicts = true
			break
		}
	}

	return WorkTreeStatus{Clean: false, HasConflicts: hasConflicts}, nil
}
