package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestCalculateColumnWidthsDistributesExtraSpace(t *testing.T) {
	repos := []*git.Repository{
		testRepoWithBranch("repository-alpha", "feature/abc"),
		testRepoWithBranch("beta", "main"),
	}
	widths := calculateColumnWidths(60, repos)

	assert.Equal(t, 25, widths.repo, "repo width")
	assert.Equal(t, 18, widths.branch, "branch width")
	assert.Equal(t, 13, widths.commitMsg, "commit width")
}

func TestCalculateColumnWidthsRespectsMinimums(t *testing.T) {
	repos := []*git.Repository{
		testRepoWithBranch("example", "long-branch-name"),
	}
	totalWidth := 31
	widths := calculateColumnWidths(totalWidth, repos)

	require.GreaterOrEqual(t, widths.repo, repoColumnMinWidth, "repo width below minimum")
	assert.Equal(t, branchColumnMinWidth, widths.branch, "branch width")
	require.GreaterOrEqual(t, widths.commitMsg, commitColumnMinWidth, "commit width below minimum")
	assert.Equal(t, totalWidth-4, widths.repo+widths.branch+widths.commitMsg, "width sum")
}

func TestCalculateColumnWidthsShrinksRepoBeforeBranch(t *testing.T) {
	repos := []*git.Repository{
		testRepoWithBranch("repository-alpha", "feature/abcdefghijklmnop"),
	}

	widths := calculateColumnWidths(48, repos)

	assert.Equal(t, repoColumnMinWidth, widths.repo, "repo width")
	assert.Equal(t, 18, widths.branch, "branch width")
	assert.Equal(t, commitColumnMinWidth, widths.commitMsg, "commit width")
}

func TestCalculateColumnWidthsShrinksBranchAfterRepoHitsMinimum(t *testing.T) {
	repos := []*git.Repository{
		testRepoWithBranch("repository-alpha", "feature/abcdefghijklmnop"),
	}

	widths := calculateColumnWidths(46, repos)

	assert.Equal(t, repoColumnMinWidth, widths.repo, "repo width")
	assert.Equal(t, 16, widths.branch, "branch width")
	assert.Equal(t, commitColumnMinWidth, widths.commitMsg, "commit width")
}

func TestCalculateColumnWidthsHandlesNarrowTables(t *testing.T) {
	widths := calculateColumnWidths(3, nil)
	assert.Equal(t, 0, widths.repo)
	assert.Equal(t, 0, widths.branch)
	assert.Equal(t, 0, widths.commitMsg)
}

func TestCalculateColumnWidthsIncludesAgePadding(t *testing.T) {
	repo := testRepoWithBranch("example", "main")
	repo.State.Branch.State = &git.BranchState{
		Commit: &git.Commit{
			Commiter: &git.Contributor{When: time.Now().Add(-48 * time.Hour)},
		},
	}

	widths := calculateColumnWidths(130, []*git.Repository{repo})

	assert.Equal(t, 4, widths.age)
	assert.Equal(t, 125, widths.repo+widths.branch+widths.commitMsg+widths.age)
}

func TestTerminalTooSmallUsesUpdatedMinimumWidth(t *testing.T) {
	model := Model{height: minTerminalHeight}

	model.width = minTerminalWidth - 1
	assert.True(t, model.terminalTooSmall())

	model.width = minTerminalWidth
	assert.False(t, model.terminalTooSmall())
}

func TestPanelViewportSizeLargeBudget(t *testing.T) {
	model := Model{width: 120, height: 30}
	assert.Equal(t, 10, model.panelViewportSize(10))
}

func TestPanelViewportSizeMinimalSpace(t *testing.T) {
	model := Model{width: 80, height: 12}
	assert.Equal(t, 1, model.panelViewportSize(5))
}

func TestPanelViewportSizeNoItems(t *testing.T) {
	model := Model{width: 80, height: 20}
	assert.Equal(t, 0, model.panelViewportSize(0))
}

func TestPanelViewportSizeClampsToBudget(t *testing.T) {
	model := Model{width: 80, height: 15}
	assert.Equal(t, 3, model.panelViewportSize(9))
}

func TestCommitAgeStringUsesDaysBeforeWeeks(t *testing.T) {
	now := time.Now()

	assert.Equal(t, "23h", commitAgeString(now.Add(-23*time.Hour)))
	assert.Equal(t, "1d", commitAgeString(now.Add(-24*time.Hour)))
	assert.Equal(t, "6d", commitAgeString(now.Add(-(6*24)*time.Hour-time.Hour)))
	assert.Equal(t, "1w", commitAgeString(now.Add(-(7 * 24 * time.Hour))))
}

func TestCommitAgeStringUsesWeeksMonthsAndYears(t *testing.T) {
	now := time.Now()

	assert.Equal(t, "2w", commitAgeString(now.Add(-(14 * 24 * time.Hour))))
	assert.Equal(t, "1mo", commitAgeString(now.Add(-(30 * 24 * time.Hour))))
	assert.Equal(t, "1y", commitAgeString(now.Add(-(365 * 24 * time.Hour))))
}

func TestRenderRepositoryLineAgeColumnHasLeftAndRightPadding(t *testing.T) {
	repo := testRepoWithBranch("example", "main")
	repo.State.Branch.State = &git.BranchState{
		Commit: &git.Commit{
			Commiter: &git.Contributor{When: time.Now().Add(-48 * time.Hour)},
		},
	}

	model := Model{styles: DefaultStyles()}
	line := ansi.Strip(model.renderRepositoryLine(repo, false, columnWidths{
		repo:      20,
		branch:    12,
		commitMsg: 18,
		age:       4,
	}))

	parts := strings.Split(line, "│")
	require.GreaterOrEqual(t, len(parts), 6)
	assert.Equal(t, " 2d ", parts[4])
}

func TestRenderOverviewFillsEmptyRowsWithAgeColumnBorder(t *testing.T) {
	repo := testRepoWithBranch("example", "main")
	repo.State.Branch.State = &git.BranchState{
		Commit: &git.Commit{
			Commiter: &git.Contributor{When: time.Now().Add(-48 * time.Hour)},
		},
	}

	model := Model{
		repositories: []*git.Repository{repo},
		width:        130,
		height:       8,
		styles:       DefaultStyles(),
	}

	lines := strings.Split(ansi.Strip(model.renderOverview()), "\n")
	require.GreaterOrEqual(t, len(lines), 6)

	parts := strings.Split(lines[3], "│")
	require.Len(t, parts, 6)
	assert.Equal(t, strings.Repeat(" ", 4), parts[4])
}

func testRepoWithBranch(name, branch string) *git.Repository {
	repo := &git.Repository{
		Name:  name,
		State: &git.RepositoryState{},
	}
	if branch != "" {
		repo.State.Branch = &git.Branch{Name: branch}
	}
	return repo
}
