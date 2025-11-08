package tui

import (
	"testing"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestCalculateColumnWidthsDistributesExtraSpace(t *testing.T) {
	repos := []*git.Repository{
		testRepoWithBranch("repository-alpha", "feature/abc"),
		testRepoWithBranch("beta", "main"),
	}
	widths := calculateColumnWidths(60, repos)

	if widths.repo != 25 {
		t.Fatalf("repo width unexpected: got %d want %d", widths.repo, 25)
	}
	if widths.branch != 18 {
		t.Fatalf("branch width unexpected: got %d want %d", widths.branch, 18)
	}
	if widths.commitMsg != 13 {
		t.Fatalf("commit width unexpected: got %d want %d", widths.commitMsg, 13)
	}
}

func TestCalculateColumnWidthsRespectsMinimums(t *testing.T) {
	repos := []*git.Repository{
		testRepoWithBranch("example", "long-branch-name"),
	}
	totalWidth := 30
	widths := calculateColumnWidths(totalWidth, repos)

	if widths.branch != 1 {
		t.Fatalf("branch width unexpected: got %d want %d", widths.branch, 1)
	}
	if widths.commitMsg < commitColumnMinWidth {
		t.Fatalf("commit width below minimum: got %d", widths.commitMsg)
	}
	if sum := widths.repo + widths.branch + widths.commitMsg; sum != totalWidth-4 {
		t.Fatalf("unexpected width sum: got %d want %d", sum, totalWidth-4)
	}
}

func TestCalculateColumnWidthsHandlesNarrowTables(t *testing.T) {
	widths := calculateColumnWidths(3, nil)
	if widths.repo != 0 || widths.branch != 0 || widths.commitMsg != 0 {
		t.Fatalf("expected zero widths for narrow table, got %+v", widths)
	}
}

func TestPanelViewportSizeLargeBudget(t *testing.T) {
	model := Model{height: 20}
	got := model.panelViewportSize(10)
	if got != 10 {
		t.Fatalf("unexpected viewport size: got %d want %d", got, 10)
	}
}

func TestPanelViewportSizeMinimalSpace(t *testing.T) {
	model := Model{height: 8}
	got := model.panelViewportSize(5)
	if got != 1 {
		t.Fatalf("unexpected viewport size: got %d want %d", got, 1)
	}
}

func TestPanelViewportSizeNoItems(t *testing.T) {
	model := Model{height: 12}
	got := model.panelViewportSize(0)
	if got != 0 {
		t.Fatalf("unexpected viewport size: got %d want %d", got, 0)
	}
}

func TestPanelViewportSizeClampsToBudget(t *testing.T) {
	model := Model{height: 10}
	got := model.panelViewportSize(9)
	if got != 3 {
		t.Fatalf("unexpected viewport size: got %d want %d", got, 3)
	}
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
