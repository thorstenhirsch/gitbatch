package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
	worktreeDiff  worktreeDiffEntry
	worktreeSync  worktreeSyncEntry
}

type worktreeDiffEntry struct {
	key       string
	content   string
	plain     string
	width     int
	checkedAt time.Time
}

type worktreeSyncEntry struct {
	primaryHead string
	currentHead string
	suffix      string
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
	if msg, hash, ok := linkedWorktreeCommitSummary(r); ok {
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
	if r != nil && r.IsLinkedWorktree() {
		return ""
	}

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

func linkedWorktreeCommitSummary(r *git.Repository) (string, plumbing.Hash, bool) {
	if r == nil || !r.IsLinkedWorktree() {
		return "", plumbing.Hash{}, false
	}

	commitObj, _, err := r.LatestCommitAheadOfPrimary()
	if err != nil || commitObj == nil {
		return "", plumbing.Hash{}, false
	}
	return firstLine(commitObj.Message), commitObj.Hash, true
}

func (m *Model) worktreeBranchContent(row overviewRow) string {
	content := row.worktreeLabel()
	repo := row.actionRepository()
	if repo == nil {
		return content
	}
	if repo.IsLinkedWorktree() {
		return content + m.linkedWorktreeSyncSuffix(repo)
	}
	if repo.State == nil {
		return content
	}
	return content + syncSuffix(repo.State.Branch)
}

func (m *Model) linkedWorktreeSyncSuffix(r *git.Repository) string {
	if r == nil || !r.IsLinkedWorktree() {
		return ""
	}

	primary := r.PrimaryWorktree()
	current := r.CurrentWorktree()
	if primary == nil || current == nil {
		return ""
	}

	primaryHead := strings.TrimSpace(primary.Head)
	currentHead := strings.TrimSpace(current.Head)
	if primaryHead == "" || currentHead == "" {
		return ""
	}

	entry := m.displayEntry(r.RepoID)
	if entry.worktreeSync.primaryHead == primaryHead && entry.worktreeSync.currentHead == currentHead {
		return entry.worktreeSync.suffix
	}

	_, ahead, err := r.LatestCommitAheadOfPrimary()
	suffix := ""
	if err == nil && ahead > 0 {
		suffix = fmt.Sprintf(" %s%d", pushable, ahead)
	}

	entry.worktreeSync.primaryHead = primaryHead
	entry.worktreeSync.currentHead = currentHead
	entry.worktreeSync.suffix = suffix
	return suffix
}

func (m *Model) worktreeDiffContent(r *git.Repository, selected bool) (string, int) {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return "", 0
	}

	entry := m.displayEntry(r.RepoID)
	key := worktreeDiffCacheKey(r)
	now := time.Now()
	if entry.worktreeDiff.key == key && now.Sub(entry.worktreeDiff.checkedAt) < time.Second {
		if selected {
			return entry.worktreeDiff.plain, entry.worktreeDiff.width
		}
		return entry.worktreeDiff.content, entry.worktreeDiff.width
	}

	insertions, deletions, ok := 0, 0, false
	if repoIsDirty(r) || repoHasLocalChanges(r) {
		insertions, deletions, ok = worktreeDiffStats(r)
	}
	if !ok {
		insertions, deletions, ok = parseWorktreeDiffMessage(r.State.Message)
	}
	content := ""
	plain := ""
	width := 0
	if ok {
		plain = fmt.Sprintf("+%d -%d", insertions, deletions)
		content = worktreeDiffAdditionStyle.Render(fmt.Sprintf("+%d", insertions)) + " " +
			worktreeDiffDeletionStyle.Render(fmt.Sprintf("-%d", deletions))
		width = lipgloss.Width(plain)
	}

	entry.worktreeDiff.key = key
	entry.worktreeDiff.content = content
	entry.worktreeDiff.plain = plain
	entry.worktreeDiff.width = width
	entry.worktreeDiff.checkedAt = now
	if selected {
		return plain, width
	}
	return content, width
}

func worktreeDiffCacheKey(r *git.Repository) string {
	head := ""
	message := ""
	if r != nil && r.State != nil && r.State.Branch != nil && r.State.Branch.Reference != nil {
		head = r.State.Branch.Reference.Hash().String()
	}
	if r != nil && r.State != nil {
		message = strings.TrimSpace(r.State.Message)
	}
	return fmt.Sprintf("%s|%t|%t|%s", head, repoIsDirty(r), repoHasLocalChanges(r), message)
}

func worktreeDiffStats(r *git.Repository) (int, int, bool) {
	if r == nil {
		return 0, 0, false
	}

	totalInsertions := 0
	totalDeletions := 0
	found := false
	for _, args := range [][]string{
		{"diff", "--shortstat", "--cached"},
		{"diff", "--shortstat"},
	} {
		out, err := statusGitCommand(r.AbsPath, args...)
		if err != nil || out == "" {
			continue
		}
		insertions, deletions := parseShortStat(out)
		if insertions == 0 && deletions == 0 {
			continue
		}
		totalInsertions += insertions
		totalDeletions += deletions
		found = true
	}
	return totalInsertions, totalDeletions, found
}

func parseShortStat(stat string) (int, int) {
	insertions := 0
	deletions := 0

	if match := worktreeDiffInsertionsRE.FindStringSubmatch(stat); len(match) == 2 {
		insertions, _ = strconv.Atoi(match[1])
	}
	if match := worktreeDiffDeletionsRE.FindStringSubmatch(stat); len(match) == 2 {
		deletions, _ = strconv.Atoi(match[1])
	}
	return insertions, deletions
}

func parseWorktreeDiffMessage(message string) (int, int, bool) {
	insertions, deletions := parseShortStat(message)
	if insertions == 0 && deletions == 0 {
		return 0, 0, false
	}
	return insertions, deletions, true
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
