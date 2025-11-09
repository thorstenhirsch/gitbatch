package tui

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/load"
)

// loadRepositoriesCmd returns a command that loads repositories
func loadRepositoriesCmd(directories []string) tea.Cmd {
	return func() tea.Msg {
		if len(directories) == 0 {
			return errMsg{err: fmt.Errorf("no directories provided")}
		}

		// Load repositories synchronously in this goroutine
		// (which is already async relative to the UI)
		repos, err := load.SyncLoad(directories)
		if err != nil {
			return errMsg{err: err}
		}

		return repositoriesLoadedMsg{repos: repos}
	}
}

// tickCmd returns a command that sends a tick message after a delay
func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return jobCompletedMsg{}
	})
}

// isLazygitAvailable checks if lazygit is in PATH
func isLazygitAvailable() bool {
	_, err := exec.LookPath("lazygit")
	return err == nil
}

func refreshRepoStateCmd(repo *git.Repository) tea.Cmd {
	return func() tea.Msg {
		if repo == nil {
			return repoRefreshResultMsg{}
		}
		if err := command.ScheduleRepositoryRefresh(repo, nil); err != nil {
			return repoRefreshResultMsg{repo: repo, err: err}
		}
		return repoRefreshResultMsg{repo: repo}
	}
}
