package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestOverviewRows_WorktreeModeUsesPrimaryWorktreeAsFirstSelectableRow(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
	}

	rows := model.overviewRows()
	require.Len(t, rows, 2)
	require.Equal(t, overviewWorktreeRow, rows[0].kind)
	require.True(t, rows[0].selectable())
	require.Same(t, primary, rows[0].repository())
	require.Same(t, primary, rows[0].actionRepository())
	require.Equal(t, "[main]", rows[0].worktreeLabel())
	require.Equal(t, overviewWorktreeRow, rows[1].kind)
	require.True(t, rows[1].selectable())
	require.Same(t, linked, rows[1].repository())
	require.Same(t, linked, rows[1].actionRepository())
	require.Equal(t, "feature", rows[1].worktreeLabel())
}

func TestOverviewRows_WorktreeModeWithoutWorktreesRendersMainLabel(t *testing.T) {
	repo := testRepoWithoutWorktrees("app", "/repos/app", "/repos/app/.git", "feature/from-state")

	model := Model{
		repositories: []*git.Repository{repo},
		worktreeMode: true,
		width:        80,
		height:       8,
		styles:       DefaultStyles(),
	}

	rows := model.overviewRows()
	require.Len(t, rows, 1)
	require.True(t, rows[0].selectable())
	require.Same(t, repo, rows[0].repository())
	require.Same(t, repo, rows[0].actionRepository())

	rendered := model.renderOverview()
	require.Contains(t, rendered, "[main]")
	require.NotContains(t, rendered, "feature/from-state")
}

func TestOverviewRows_WorktreeModeRendersWorktreeLabelWithoutBranchSuffix(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
		width:        80,
		height:       8,
		styles:       DefaultStyles(),
	}

	rendered := model.renderOverview()
	require.Contains(t, rendered, "feature")
	require.NotContains(t, rendered, "feature (feature/demo)")
}

func TestRenderWorktreeLine_PrimaryWorktreeShowsDiffAndMainLabel(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)
	primary.State.Message = "1 file changed, 3 insertions(+), 1 deletion(-)"

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
		styles:       DefaultStyles(),
	}

	rows := model.overviewRows()
	require.Len(t, rows, 2)

	repoCol, worktreeCol, _ := renderedColumns(t, ansi.Strip(model.renderWorktreeLine(rows[0], false, columnWidths{
		repo:      28,
		branch:    18,
		commitMsg: 24,
	})))
	require.Contains(t, repoCol, "app")
	require.Contains(t, repoCol, "+3 -1")
	require.Equal(t, "[main]", worktreeCol)
}

func TestCommitContentForRepo_LinkedWorktreeShowsLatestCommitAheadOfMain(t *testing.T) {
	basePath := initLinkedWorktreeRepoForTUITest(t)
	linkedPath := filepath.Join(filepath.Dir(basePath), "feature-worktree")

	require.NoError(t, os.WriteFile(filepath.Join(linkedPath, "feature.txt"), []byte("feature"), 0o644))
	runGitCommandForTUITest(t, linkedPath, "add", "feature.txt")
	runGitCommandForTUITest(t, linkedPath, "commit", "-m", "Feature ahead of main")

	repo, err := git.InitializeRepo(linkedPath)
	require.NoError(t, err)
	model := Model{}

	content := model.commitContentForRepo(repo)
	require.Contains(t, content, "Feature ahead of main")
}

func TestRenderWorktreeLine_LinkedWorktreeShowsAheadIndicatorAndKeepsTrimmedLabel(t *testing.T) {
	basePath, linkedPath := initNamedLinkedWorktreeRepoForTUITest(t, "app", "app-feature", "feature/demo")

	require.NoError(t, os.WriteFile(filepath.Join(linkedPath, "feature.txt"), []byte("feature"), 0o644))
	runGitCommandForTUITest(t, linkedPath, "add", "feature.txt")
	runGitCommandForTUITest(t, linkedPath, "commit", "-m", "Feature ahead of main")

	primary, err := git.InitializeRepo(basePath)
	require.NoError(t, err)
	linked, err := git.InitializeRepo(linkedPath)
	require.NoError(t, err)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
		styles:       DefaultStyles(),
	}

	rows := model.overviewRows()
	require.Len(t, rows, 2)

	var linkedRow overviewRow
	for _, row := range rows {
		if row.worktree != nil && !row.worktree.IsPrimary {
			linkedRow = row
			break
		}
	}
	require.NotNil(t, linkedRow.worktree)

	_, worktreeCol, commitCol := renderedColumns(t, ansi.Strip(model.renderWorktreeLine(linkedRow, false, columnWidths{
		repo:      28,
		branch:    18,
		commitMsg: 28,
	})))
	require.Contains(t, worktreeCol, "feature")
	require.Contains(t, worktreeCol, "↖1")
	require.NotContains(t, worktreeCol, "feature/demo")
	require.Contains(t, commitCol, "Feature ahead of main")
}

func TestRenderWorktreeLine_SelectedPrimaryWorktreeUsesPlainDiffContent(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	primary.State.Branch.HasLocalChanges = true
	primary.State.Message = "1 file changed, 3 insertions(+), 1 deletion(-)"

	model := Model{
		repositories: []*git.Repository{primary},
		styles:       DefaultStyles(),
	}

	unselected, width := model.worktreeDiffContent(primary, false)
	selected, selectedWidth := model.worktreeDiffContent(primary, true)

	require.Equal(t, lipgloss.Width("+3 -1"), width)
	require.Equal(t, width, selectedWidth)
	require.Equal(t, "+3 -1", ansi.Strip(unselected))
	require.Equal(t, "+3 -1", selected)
}

func TestRenderStatusBar_LinkedWorktreeShowsNeutralWorktreeLabel(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)
	linked.State.Branch.HasLocalChanges = true

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
		cursor:       1,
		mode:         pushMode,
		width:        100,
		styles:       DefaultStyles(),
	}

	statusBar := ansi.Strip(model.renderStatusBar())
	require.Contains(t, statusBar, "worktree: feature")
	require.Contains(t, statusBar, "n worktree")
	require.Contains(t, statusBar, "d delete")
	require.NotContains(t, statusBar, "space: tag")
	require.NotContains(t, statusBar, "f fetch")
	require.NotContains(t, statusBar, "p pull")
	require.NotContains(t, statusBar, "P push")
	require.NotContains(t, statusBar, "m: switch")
}

func TestRenderStatusBar_PrimaryWorktreeShowsNewWorktreeHintWithoutDelete(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
		cursor:       0,
		mode:         pullMode,
		width:        100,
		styles:       DefaultStyles(),
	}

	statusBar := ansi.Strip(model.renderStatusBar())
	require.Contains(t, statusBar, "n worktree")
	require.Contains(t, statusBar, "W branches")
	require.NotContains(t, statusBar, "d delete")
}

func TestRepoIsActionable_LinkedWorktreeIsDisabled(t *testing.T) {
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)
	require.False(t, repoIsActionable(linked))
}

func TestCycleMode_LinkedWorktreeSelectionDoesNothing(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
		cursor:       1,
		mode:         pullMode,
	}

	model.cycleMode()

	require.Equal(t, pullMode, model.mode)
}

func TestOverviewRows_WorktreeModePromotesRepositoryAfterWorktreeCreation(t *testing.T) {
	repo := testRepoWithoutWorktrees("app", "/repos/app", "/repos/app/.git", "main")

	model := Model{
		repositories: []*git.Repository{repo},
		worktreeMode: true,
	}

	row, ok := model.currentOverviewRow()
	require.True(t, ok)
	require.Equal(t, overviewRepositoryRow, row.kind)
	require.Same(t, repo, model.currentWorktreeCommandRepository())

	repo.Worktrees = []*git.Worktree{
		{
			Path:       "/repos/app",
			BranchName: "main",
			IsPrimary:  true,
			IsCurrent:  true,
		},
		{
			Path:       "/worktrees/app-feature",
			BranchName: "feature/demo",
		},
	}

	rows := model.overviewRows()
	require.Len(t, rows, 2)
	require.Equal(t, overviewWorktreeRow, rows[0].kind)
	require.True(t, rows[0].selectable())
	require.Equal(t, "[main]", rows[0].worktreeLabel())
	require.Equal(t, overviewWorktreeRow, rows[1].kind)
	require.True(t, rows[1].selectable())
	require.Equal(t, "feature", rows[1].worktreeLabel())
}

func TestToggleWorktreeMode_SelectsCurrentWorktreeRow(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		cursor:       1,
	}

	model.toggleWorktreeMode()

	require.True(t, model.worktreeMode)
	row, ok := model.currentOverviewRow()
	require.True(t, ok)
	require.Equal(t, overviewWorktreeRow, row.kind)
	require.NotNil(t, row.worktree)
	require.Equal(t, normalizeOverviewPath("/worktrees/app-feature"), normalizeOverviewPath(row.worktree.Path))
}

func TestDeleteSelectedWorktreeCmd_BlocksMainWorktree(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)

	model := Model{
		repositories: []*git.Repository{primary},
		worktreeMode: true,
		cursor:       0,
	}

	cmd := model.deleteSelectedWorktreeCmd()
	require.Nil(t, cmd)
	require.Equal(t, "cannot delete [main] worktree", primary.State.Message)
}

func TestOverviewRow_WorktreeLabelTrimsRepositoryPrefix(t *testing.T) {
	repo := testRepoWithoutWorktrees("foo", "/repos/foo", "/repos/foo/.git", "main")

	row := overviewRow{
		kind:        overviewWorktreeRow,
		repo:        repo,
		displayRepo: repo,
		worktree: &git.Worktree{
			Path:       "/worktrees/foo.bar",
			BranchName: "feature/demo",
		},
	}

	require.Equal(t, "bar", row.worktreeLabel())
}

func TestFirstSelectableIndex_WorktreeModeStartsAtPrimaryWorktreeRow(t *testing.T) {
	primary := testRepoWithWorktree("app", "/repos/app", "/repos/app/.git", "main", true)
	linked := testRepoWithWorktree("app-feature", "/worktrees/app-feature", "/repos/app/.git", "feature/demo", false)

	model := Model{
		repositories: []*git.Repository{primary, linked},
		worktreeMode: true,
	}

	require.Equal(t, 0, model.firstSelectableIndex())
	require.Equal(t, 1, model.lastSelectableIndex())
}

func testRepoWithoutWorktrees(name, absPath, commonGitDir, branch string) *git.Repository {
	return &git.Repository{
		Name:         name,
		AbsPath:      absPath,
		CommonGitDir: commonGitDir,
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: branch},
		},
	}
}

func testRepoWithWorktree(name, absPath, commonGitDir, branch string, primary bool) *git.Repository {
	worktrees := []*git.Worktree{
		{
			Path:       "/repos/app",
			BranchName: "main",
			IsPrimary:  true,
			IsCurrent:  primary,
		},
	}
	if !primary {
		worktrees = append(worktrees, &git.Worktree{
			Path:       "/worktrees/app-feature",
			BranchName: "feature/demo",
			IsCurrent:  true,
		})
	} else {
		worktrees = append(worktrees, &git.Worktree{
			Path:       "/worktrees/app-feature",
			BranchName: "feature/demo",
		})
	}

	repo := &git.Repository{
		Name:         name,
		AbsPath:      absPath,
		CommonGitDir: commonGitDir,
		Worktrees:    worktrees,
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: branch},
		},
	}
	return repo
}

func initLinkedWorktreeRepoForTUITest(t *testing.T) string {
	t.Helper()

	basePath, _ := initNamedLinkedWorktreeRepoForTUITest(t, "repo", "feature-worktree", "feature/worktree")
	return basePath
}

func initNamedLinkedWorktreeRepoForTUITest(t *testing.T, baseName, linkedName, branchName string) (string, string) {
	t.Helper()

	tempRoot := t.TempDir()
	basePath := filepath.Join(tempRoot, baseName)
	remotePath := filepath.Join(tempRoot, "remote.git")
	linkedPath := filepath.Join(tempRoot, linkedName)
	require.NoError(t, os.MkdirAll(basePath, 0o755))
	require.NoError(t, os.MkdirAll(remotePath, 0o755))

	runGitCommandForTUITest(t, basePath, "init", "--initial-branch=main")
	runGitCommandForTUITest(t, remotePath, "init", "--bare")
	runGitCommandForTUITest(t, basePath, "config", "user.email", "test@example.com")
	runGitCommandForTUITest(t, basePath, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "README.md"), []byte("hello"), 0o644))
	runGitCommandForTUITest(t, basePath, "add", "README.md")
	runGitCommandForTUITest(t, basePath, "commit", "-m", "initial commit")
	runGitCommandForTUITest(t, basePath, "remote", "add", "origin", remotePath)
	runGitCommandForTUITest(t, basePath, "push", "-u", "origin", "main")

	runGitCommandForTUITest(t, basePath, "worktree", "add", "-b", branchName, linkedPath)

	return basePath, linkedPath
}

func renderedColumns(t *testing.T, line string) (string, string, string) {
	t.Helper()

	parts := strings.Split(line, "│")
	require.GreaterOrEqual(t, len(parts), 5, "expected table row with three columns: %q", line)
	return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3])
}

func runGitCommandForTUITest(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}
