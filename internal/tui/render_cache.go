package tui

import (
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// repoDisplayEntry caches pre-rendered commit content for a single repository,
// keyed by commit hash. View() is called on every state tick; without this
// cache each render hits go-git (disk I/O for Tags/TagObject/CommitObject)
// across every visible repo, which dominates frame time for users with many
// repos or many tags.
//
// The cache is keyed by the branch reference hash. A tag added externally
// without changing HEAD will not invalidate — the user can press R (which
// triggers a repository refresh and eventually re-populates) or the next
// fsnotify-driven refresh will update it. This trade-off is intentional:
// correctness in the overwhelmingly common case (HEAD changes when commits
// arrive) and no disk I/O on the hot path.
type repoDisplayEntry struct {
	headHash      plumbing.Hash
	headContent   string
	branchContent map[plumbing.Hash]string
}

func (m *Model) displayEntry(repoID string) *repoDisplayEntry {
	if m.displayCache == nil {
		m.displayCache = make(map[string]*repoDisplayEntry)
	}
	entry, ok := m.displayCache[repoID]
	if !ok {
		entry = &repoDisplayEntry{branchContent: make(map[plumbing.Hash]string)}
		m.displayCache[repoID] = entry
	}
	return entry
}

// commitContentForRepo returns the pre-rendered "[tags] commit-message" line
// for r's HEAD branch. On a fail state it returns the sticky error message.
// Cache miss triggers a single-shot disk read via go-git; hits are O(1).
func (m *Model) commitContentForRepo(r *git.Repository) string {
	if r == nil {
		return ""
	}
	if r.WorkStatus() == git.Fail && r.State != nil && r.State.Message != "" {
		return singleLineMessage(r.State.Message)
	}
	if r.State == nil || r.State.Branch == nil || r.State.Branch.Reference == nil {
		return ""
	}

	entry := m.displayEntry(r.RepoID)
	hash := r.State.Branch.Reference.Hash()
	if entry.headHash == hash && entry.headContent != "" {
		return entry.headContent
	}

	content := computeCommitContent(r)
	entry.headHash = hash
	entry.headContent = content
	return content
}

// branchCommitContent returns the pre-rendered commit message for a non-HEAD
// branch in the expanded-branch view. Separate from commitContentForRepo so
// expansions don't collide with the repo-level HEAD cache.
func (m *Model) branchCommitContent(r *git.Repository, branch *git.Branch) string {
	if r == nil || branch == nil || branch.Reference == nil {
		return ""
	}

	entry := m.displayEntry(r.RepoID)
	hash := branch.Reference.Hash()
	if cached, ok := entry.branchContent[hash]; ok {
		return cached
	}

	msg := computeBranchCommitMessage(r, branch)
	entry.branchContent[hash] = msg
	return msg
}

// computeCommitContent is the slow path — walks go-git refs/tags to find tags
// pointing at HEAD and reads the commit object when branch.State.Commit is
// not populated. Called on cache miss only.
func computeCommitContent(r *git.Repository) string {
	msg, hash := commitSummary(r)
	tags := collectTags(r, hash)
	parts := make([]string, 0, 2)
	if len(tags) > 0 {
		parts = append(parts, "["+strings.Join(tags, ", ")+"]")
	}
	if msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, " ")
}

func computeBranchCommitMessage(r *git.Repository, branch *git.Branch) string {
	if branch.State != nil && branch.State.Commit != nil {
		if branch.State.Commit.C != nil {
			return firstLine(branch.State.Commit.C.Message)
		}
		if branch.State.Commit.Message != "" {
			return firstLine(branch.State.Commit.Message)
		}
	}
	commitObj, err := r.Repo.CommitObject(branch.Reference.Hash())
	if err != nil {
		return ""
	}
	return firstLine(commitObj.Message)
}

