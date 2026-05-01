package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestOpenBranchPrompt_UsesCurrentRepositoryWhenNothingTagged(t *testing.T) {
	repo1 := testBranchPromptRepo("alpha", "main")
	repo2 := testBranchPromptRepo("beta", "main")

	model := Model{
		repositories: []*git.Repository{repo1, repo2},
		cursor:       1,
	}

	model.openBranchPrompt()

	require.True(t, model.branchPromptActive)
	require.Equal(t, []*git.Repository{repo2}, model.branchPromptRepos)
}

func TestHandleBranchPanelKey_NOpensBranchPromptWithoutCommonBranches(t *testing.T) {
	repo1 := testBranchPromptRepo("alpha", "main")
	repo2 := testBranchPromptRepo("beta", "develop")
	repo1.SetWorkStatusSilent(git.Queued)
	repo2.SetWorkStatusSilent(git.Queued)

	model := Model{
		repositories: []*git.Repository{repo1, repo2},
		sidePanel:    BranchPanel,
	}

	updated, cmd := model.handleBranchPanelKey("n")

	require.Same(t, &model, updated)
	require.Nil(t, cmd)
	require.True(t, model.branchPromptActive)
	require.Equal(t, []*git.Repository{repo1, repo2}, model.branchPromptRepos)
}

func TestCreateBranchCmd_CreatesAndChecksOutBranchInAllTargets(t *testing.T) {
	repo1 := initBranchCreationRepo(t, "alpha")
	repo2 := initBranchCreationRepo(t, "beta")
	model := Model{}

	cmd := model.createBranchCmd([]*git.Repository{repo1, repo2}, "feature/demo")
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, repoActionResultMsg{}, msg)

	require.Equal(t, "feature/demo", currentBranchName(t, repo1.AbsPath))
	require.Equal(t, "feature/demo", currentBranchName(t, repo2.AbsPath))
}

func testBranchPromptRepo(name, currentBranch string) *git.Repository {
	repo := &git.Repository{
		Name: name,
		Branches: []*git.Branch{
			{Name: currentBranch},
		},
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: currentBranch},
		},
	}
	repo.SetWorkStatusSilent(git.Available)
	return repo
}

func initBranchCreationRepo(t *testing.T, name string) *git.Repository {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, name)
	remotePath := filepath.Join(root, name+".git")
	require.NoError(t, os.MkdirAll(repoPath, 0o755))
	require.NoError(t, os.MkdirAll(remotePath, 0o755))

	runBranchTestGit(t, repoPath, "init", "--initial-branch=main")
	runBranchTestGit(t, remotePath, "init", "--bare")
	runBranchTestGit(t, repoPath, "config", "user.email", "test@example.com")
	runBranchTestGit(t, repoPath, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello"), 0o644))
	runBranchTestGit(t, repoPath, "add", "README.md")
	runBranchTestGit(t, repoPath, "commit", "-m", "initial commit")
	runBranchTestGit(t, repoPath, "remote", "add", "origin", remotePath)
	runBranchTestGit(t, repoPath, "push", "-u", "origin", "main")

	repo, err := git.InitializeRepo(repoPath)
	require.NoError(t, err)
	return repo
}

func currentBranchName(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git branch --show-current failed: %s", string(output))
	return strings.TrimSpace(string(output))
}

func runBranchTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}
