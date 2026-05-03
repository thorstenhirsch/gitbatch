package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree describes one entry returned by `git worktree list --porcelain`.
type Worktree struct {
	Path           string
	BranchRef      string
	BranchName     string
	Head           string
	IsPrimary      bool
	IsCurrent      bool
	IsDetached     bool
	IsBare         bool
	IsLocked       bool
	LockReason     string
	IsPrunable     bool
	PrunableReason string
}

// WorktreeAddOptions controls creation of a linked worktree.
type WorktreeAddOptions struct {
	Path       string
	BranchName string
	StartPoint string // only applies when NewBranch is true
	Force      bool
	NewBranch  bool // when true, creates a new branch (-b); when false, checks out an existing branch
}

// RepositoryStateProfile groups repositories by the status rules that apply to
// them during state evaluation.
type RepositoryStateProfile uint8

const (
	RepositoryStateProfileRegular RepositoryStateProfile = iota
	RepositoryStateProfileLinkedWorktree
)

// DisplayName returns a compact label for the worktree path.
func (w *Worktree) DisplayName() string {
	if w == nil {
		return ""
	}
	base := filepath.Base(w.Path)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return w.Path
	}
	return base
}

// FamilyKey returns the stable key used to group repositories that belong to the same worktree family.
func (r *Repository) FamilyKey() string {
	if r == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(r.CommonGitDir) != "":
		return normalizeRepositoryPath(r.CommonGitDir)
	case strings.TrimSpace(r.GitDir) != "":
		return normalizeRepositoryPath(r.GitDir)
	default:
		return normalizeRepositoryPath(r.AbsPath)
	}
}

// StateProfile reports which state-evaluation profile applies to the repository.
func (r *Repository) StateProfile() RepositoryStateProfile {
	if r != nil {
		if current := r.CurrentWorktree(); current != nil && !current.IsPrimary {
			return RepositoryStateProfileLinkedWorktree
		}
	}
	return RepositoryStateProfileRegular
}

// IsLinkedWorktree reports whether the repository directory is a linked worktree
// rather than the primary worktree of its family.
func (r *Repository) IsLinkedWorktree() bool {
	return r.StateProfile() == RepositoryStateProfileLinkedWorktree
}

// PrimaryWorktree returns the main worktree for the repository family.
func (r *Repository) PrimaryWorktree() *Worktree {
	if r == nil {
		return nil
	}
	for _, worktree := range r.Worktrees {
		if worktree != nil && worktree.IsPrimary {
			return worktree
		}
	}
	if len(r.Worktrees) == 0 {
		return nil
	}
	return r.Worktrees[0]
}

// CurrentWorktree returns the worktree that matches the repository's current working directory.
func (r *Repository) CurrentWorktree() *Worktree {
	if r == nil {
		return nil
	}
	for _, worktree := range r.Worktrees {
		if worktree != nil && worktree.IsCurrent {
			return worktree
		}
	}
	return nil
}

func (r *Repository) loadWorktrees() error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}

	gitDir, err := r.revParsePath("--git-dir")
	if err != nil {
		return err
	}
	commonGitDir, err := r.revParsePath("--git-common-dir")
	if err != nil {
		return err
	}

	output, err := r.runGit("worktree", "list", "--porcelain")
	if err != nil {
		return err
	}

	worktrees, err := parseWorktreePorcelain(output, r.AbsPath)
	if err != nil {
		return err
	}

	r.GitDir = gitDir
	r.CommonGitDir = commonGitDir
	r.Worktrees = worktrees
	return nil
}

// CreateWorktree creates a new linked worktree on a new branch.
func (r *Repository) CreateWorktree(options WorktreeAddOptions) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	path := strings.TrimSpace(options.Path)
	if path == "" {
		return fmt.Errorf("worktree path required")
	}
	branchName := strings.TrimSpace(options.BranchName)
	if branchName == "" {
		return fmt.Errorf("worktree branch name required")
	}

	args := []string{"worktree", "add"}
	if options.Force {
		args = append(args, "--force")
	}
	if options.NewBranch {
		args = append(args, "-b", branchName, path)
		if startPoint := strings.TrimSpace(options.StartPoint); startPoint != "" {
			args = append(args, startPoint)
		}
	} else {
		args = append(args, path, branchName)
	}

	r.BeginWatchSuppress()
	defer r.EndWatchSuppress()

	if _, err := r.runGitForWorktrees(args...); err != nil {
		return err
	}
	return nil
}

// RemoveWorktree removes a linked worktree. The primary worktree cannot be removed.
func (r *Repository) RemoveWorktree(worktree *Worktree, force bool) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	if worktree == nil {
		return fmt.Errorf("worktree required")
	}
	if worktree.IsPrimary {
		return fmt.Errorf("cannot remove primary worktree")
	}
	if strings.TrimSpace(worktree.Path) == "" {
		return fmt.Errorf("worktree path required")
	}

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktree.Path)

	r.BeginWatchSuppress()
	defer r.EndWatchSuppress()

	_, err := r.runGitForWorktrees(args...)
	return err
}

// LocalBranchExists reports whether a local branch with the given name exists.
func (r *Repository) LocalBranchExists(name string) bool {
	if r == nil || name == "" {
		return false
	}
	_, err := r.runGitForWorktrees("rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// PruneWorktrees removes stale worktree metadata from .git/worktrees/.
func (r *Repository) PruneWorktrees() error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	r.BeginWatchSuppress()
	defer r.EndWatchSuppress()
	_, err := r.runGitForWorktrees("worktree", "prune")
	return err
}

// LockWorktree locks a linked worktree to prevent accidental removal.
func (r *Repository) LockWorktree(worktree *Worktree, reason string) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	if worktree == nil {
		return fmt.Errorf("worktree required")
	}
	if worktree.IsPrimary {
		return fmt.Errorf("cannot lock primary worktree")
	}
	if strings.TrimSpace(worktree.Path) == "" {
		return fmt.Errorf("worktree path required")
	}
	args := []string{"worktree", "lock"}
	if reason = strings.TrimSpace(reason); reason != "" {
		args = append(args, "--reason", reason)
	}
	args = append(args, worktree.Path)
	r.BeginWatchSuppress()
	defer r.EndWatchSuppress()
	_, err := r.runGitForWorktrees(args...)
	return err
}

// UnlockWorktree removes the lock on a linked worktree.
func (r *Repository) UnlockWorktree(worktree *Worktree) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	if worktree == nil {
		return fmt.Errorf("worktree required")
	}
	if worktree.IsPrimary {
		return fmt.Errorf("cannot unlock primary worktree")
	}
	if strings.TrimSpace(worktree.Path) == "" {
		return fmt.Errorf("worktree path required")
	}
	r.BeginWatchSuppress()
	defer r.EndWatchSuppress()
	_, err := r.runGitForWorktrees("worktree", "unlock", worktree.Path)
	return err
}

func (r *Repository) runGit(args ...string) (string, error) {
	return r.runGitInDir(r.AbsPath, args...)
}

func (r *Repository) runGitForWorktrees(args ...string) (string, error) {
	commandDir := r.AbsPath
	if primary := r.PrimaryWorktree(); primary != nil && strings.TrimSpace(primary.Path) != "" {
		commandDir = primary.Path
	}
	return r.runGitInDir(commandDir, args...)
}

func (r *Repository) runGitInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

func (r *Repository) revParsePath(flag string) (string, error) {
	output, err := r.runGit("rev-parse", "--path-format=absolute", flag)
	if err != nil {
		return "", err
	}
	return normalizeRepositoryPath(output), nil
}

func parseWorktreePorcelain(output, currentPath string) ([]*Worktree, error) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	worktrees := make([]*Worktree, 0)
	currentPath = normalizeRepositoryPath(currentPath)

	appendCurrent := func(current *Worktree) error {
		if current == nil {
			return nil
		}
		if strings.TrimSpace(current.Path) == "" {
			return fmt.Errorf("worktree entry missing path")
		}
		current.Path = normalizeRepositoryPath(current.Path)
		current.IsCurrent = current.Path == currentPath
		if current.BranchRef != "" && current.BranchName == "" {
			current.BranchName = strings.TrimPrefix(current.BranchRef, "refs/heads/")
		}
		worktrees = append(worktrees, current)
		return nil
	}

	var current *Worktree
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if err := appendCurrent(current); err != nil {
				return nil, err
			}
			current = nil
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			if err := appendCurrent(current); err != nil {
				return nil, err
			}
			current = &Worktree{Path: strings.TrimSpace(strings.TrimPrefix(line, "worktree "))}
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("unexpected worktree metadata before path: %s", line)
		}

		switch {
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			current.BranchRef = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.BranchName = strings.TrimPrefix(current.BranchRef, "refs/heads/")
		case line == "detached":
			current.IsDetached = true
		case line == "bare":
			current.IsBare = true
		case strings.HasPrefix(line, "locked"):
			current.IsLocked = true
			current.LockReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
		case strings.HasPrefix(line, "prunable"):
			current.IsPrunable = true
			current.PrunableReason = strings.TrimSpace(strings.TrimPrefix(line, "prunable"))
		}
	}
	if err := appendCurrent(current); err != nil {
		return nil, err
	}
	if len(worktrees) == 0 {
		return nil, fmt.Errorf("no worktrees found")
	}
	worktrees[0].IsPrimary = true
	return worktrees, nil
}

func normalizeRepositoryPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !filepath.IsAbs(trimmed) {
		if abs, err := filepath.Abs(trimmed); err == nil {
			trimmed = abs
		}
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(trimmed)
}
