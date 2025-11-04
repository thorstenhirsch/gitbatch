package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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
