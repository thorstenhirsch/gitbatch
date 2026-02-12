package tui

import (
	"testing"

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
	totalWidth := 30
	widths := calculateColumnWidths(totalWidth, repos)

	assert.Equal(t, 1, widths.branch, "branch width")
	require.GreaterOrEqual(t, widths.commitMsg, commitColumnMinWidth, "commit width below minimum")
	assert.Equal(t, totalWidth-4, widths.repo+widths.branch+widths.commitMsg, "width sum")
}

func TestCalculateColumnWidthsHandlesNarrowTables(t *testing.T) {
	widths := calculateColumnWidths(3, nil)
	assert.Equal(t, 0, widths.repo)
	assert.Equal(t, 0, widths.branch)
	assert.Equal(t, 0, widths.commitMsg)
}

func TestPanelViewportSizeLargeBudget(t *testing.T) {
	model := Model{height: 20}
	assert.Equal(t, 10, model.panelViewportSize(10))
}

func TestPanelViewportSizeMinimalSpace(t *testing.T) {
	model := Model{height: 8}
	assert.Equal(t, 1, model.panelViewportSize(5))
}

func TestPanelViewportSizeNoItems(t *testing.T) {
	model := Model{height: 12}
	assert.Equal(t, 0, model.panelViewportSize(0))
}

func TestPanelViewportSizeClampsToBudget(t *testing.T) {
	model := Model{height: 10}
	assert.Equal(t, 3, model.panelViewportSize(9))
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
