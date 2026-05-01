package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestHandleOverviewKeys_TTogglesRepositorySort(t *testing.T) {
	alpha := &git.Repository{Name: "alpha", ModTime: time.Unix(10, 0)}
	beta := &git.Repository{Name: "beta", ModTime: time.Unix(20, 0)}

	model := Model{
		repositories: []*git.Repository{alpha, beta},
	}

	updated, cmd := model.handleOverviewKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	require.Same(t, &model, updated)
	require.Nil(t, cmd)
	require.Equal(t, repositorySortByTime, model.sortMode)
	require.Equal(t, []*git.Repository{beta, alpha}, model.repositories)

	updated, cmd = model.handleOverviewKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	require.Same(t, &model, updated)
	require.Nil(t, cmd)
	require.Equal(t, repositorySortByName, model.sortMode)
	require.Equal(t, []*git.Repository{alpha, beta}, model.repositories)
}

func TestHandleOverviewKeys_NOpensWorktreePromptInWorktreeMode(t *testing.T) {
	repo := &git.Repository{
		Name: "alpha",
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: "main"},
		},
	}

	model := Model{
		repositories: []*git.Repository{repo},
		worktreeMode: true,
	}

	updated, cmd := model.handleOverviewKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	require.Same(t, &model, updated)
	require.Nil(t, cmd)
	require.True(t, model.worktreePromptActive)
	require.Same(t, repo, model.worktreePromptRepo)
}

func TestHandleOverviewKeys_NOpensBranchPrompt(t *testing.T) {
	repo := &git.Repository{
		Name: "alpha",
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: "main"},
		},
	}

	model := Model{
		repositories: []*git.Repository{repo},
	}

	updated, cmd := model.handleOverviewKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	require.Same(t, &model, updated)
	require.Nil(t, cmd)
	require.True(t, model.branchPromptActive)
	require.Equal(t, []*git.Repository{repo}, model.branchPromptRepos)
}

func TestRenderStatusBar_ShowsBranchHintInOverview(t *testing.T) {
	repo := &git.Repository{
		Name: "alpha",
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: "main"},
		},
	}

	model := Model{
		repositories: []*git.Repository{repo},
		mode:         pullMode,
		width:        100,
		styles:       DefaultStyles(),
	}

	statusBar := ansi.Strip(model.renderStatusBar())
	require.Contains(t, statusBar, "n branch")
	require.NotContains(t, statusBar, "n worktree")
}
