package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadWorktrees_BaseAndLinked(t *testing.T) {
	basePath := initLocalWorktreeRepo(t)
	linkedPath := filepath.Join(filepath.Dir(basePath), "feature-worktree")
	runGitCommand(t, basePath, "worktree", "add", "-b", "feature/worktree", linkedPath)

	baseRepo, err := InitializeRepo(basePath)
	require.NoError(t, err)

	require.NotEmpty(t, baseRepo.GitDir)
	require.NotEmpty(t, baseRepo.CommonGitDir)
	require.Len(t, baseRepo.Worktrees, 2)

	require.True(t, baseRepo.Worktrees[0].IsPrimary)
	require.True(t, baseRepo.Worktrees[0].IsCurrent)
	require.Equal(t, normalizeRepositoryPath(basePath), baseRepo.Worktrees[0].Path)
	require.Equal(t, "main", baseRepo.Worktrees[0].BranchName)

	require.False(t, baseRepo.Worktrees[1].IsPrimary)
	require.False(t, baseRepo.Worktrees[1].IsCurrent)
	require.Equal(t, normalizeRepositoryPath(linkedPath), baseRepo.Worktrees[1].Path)
	require.Equal(t, "feature/worktree", baseRepo.Worktrees[1].BranchName)

	linkedRepo, err := InitializeRepo(linkedPath)
	require.NoError(t, err)
	require.Equal(t, normalizeRepositoryPath(baseRepo.CommonGitDir), normalizeRepositoryPath(linkedRepo.CommonGitDir))
	require.NotEqual(t, normalizeRepositoryPath(baseRepo.GitDir), normalizeRepositoryPath(linkedRepo.GitDir))

	current := linkedRepo.CurrentWorktree()
	require.NotNil(t, current)
	require.Equal(t, normalizeRepositoryPath(linkedPath), current.Path)
	require.False(t, current.IsPrimary)
}

func TestCreateWorktree_AddsLinkedWorktree(t *testing.T) {
	basePath := initLocalWorktreeRepo(t)
	repo, err := InitializeRepo(basePath)
	require.NoError(t, err)

	newPath := filepath.Join(filepath.Dir(basePath), "new-worktree")
	err = repo.CreateWorktree(WorktreeAddOptions{
		Path:       newPath,
		BranchName: "feature/new-worktree",
	})
	require.NoError(t, err)

	require.NoError(t, repo.Refresh())
	require.Len(t, repo.Worktrees, 2)

	created := repo.Worktrees[1]
	require.Equal(t, normalizeRepositoryPath(newPath), created.Path)
	require.Equal(t, "feature/new-worktree", created.BranchName)
}

func TestCreateWorktree_FromLinkedRepoUsesPrimaryWorktree(t *testing.T) {
	basePath := initLocalWorktreeRepo(t)
	linkedPath := filepath.Join(filepath.Dir(basePath), "feature-worktree")
	runGitCommand(t, basePath, "worktree", "add", "-b", "feature/worktree", linkedPath)

	repo, err := InitializeRepo(linkedPath)
	require.NoError(t, err)

	newPath := filepath.Join(filepath.Dir(basePath), "second-worktree")
	err = repo.CreateWorktree(WorktreeAddOptions{
		Path:       newPath,
		BranchName: "feature/second-worktree",
	})
	require.NoError(t, err)

	require.NoError(t, repo.Refresh())
	require.Len(t, repo.Worktrees, 3)
	require.Equal(t, normalizeRepositoryPath(newPath), repo.Worktrees[2].Path)
}

func TestRemoveWorktree_RejectsPrimary(t *testing.T) {
	basePath := initLocalWorktreeRepo(t)
	repo, err := InitializeRepo(basePath)
	require.NoError(t, err)

	err = repo.RemoveWorktree(repo.PrimaryWorktree(), false)
	require.EqualError(t, err, "cannot remove primary worktree")
}

func TestRefreshModTime_UsesGitDirForLinkedWorktree(t *testing.T) {
	basePath := initLocalWorktreeRepo(t)
	linkedPath := filepath.Join(filepath.Dir(basePath), "feature-worktree")
	runGitCommand(t, basePath, "worktree", "add", "-b", "feature/worktree", linkedPath)

	repo, err := InitializeRepo(linkedPath)
	require.NoError(t, err)

	modTime1 := repo.RefreshModTime()
	time.Sleep(20 * time.Millisecond)

	headPath := filepath.Join(repo.GitDir, "HEAD")
	content, err := os.ReadFile(headPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(headPath, content, 0o644))

	modTime2 := repo.RefreshModTime()
	require.True(t, modTime2.After(modTime1), "worktree gitdir writes should advance modtime")
}

func initLocalWorktreeRepo(t *testing.T) string {
	t.Helper()

	tempRoot := t.TempDir()
	basePath := filepath.Join(tempRoot, "repo")
	remotePath := filepath.Join(tempRoot, "remote.git")
	require.NoError(t, os.MkdirAll(basePath, 0o755))
	require.NoError(t, os.MkdirAll(remotePath, 0o755))

	runGitCommand(t, basePath, "init", "--initial-branch=main")
	runGitCommand(t, remotePath, "init", "--bare")
	runGitCommand(t, basePath, "config", "user.email", "test@example.com")
	runGitCommand(t, basePath, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "README.md"), []byte("hello"), 0o644))
	runGitCommand(t, basePath, "add", "README.md")
	runGitCommand(t, basePath, "commit", "-m", "initial commit")
	runGitCommand(t, basePath, "remote", "add", "origin", remotePath)
	runGitCommand(t, basePath, "push", "-u", "origin", "main")

	return basePath
}

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}
