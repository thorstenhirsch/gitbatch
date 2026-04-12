package tui

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/load"
)

// loadProgressCh carries incremental load counts from the worker pool to the TUI.
var loadProgressCh = make(chan int, 256)

// loadRepositoriesCmd returns a command that loads repositories.
// When len(directories) > loadingScreenThreshold it also reports progress via
// loadProgressCh so the loading screen can show a progress bar.
func loadRepositoriesCmd(directories []string) tea.Cmd {
	return func() tea.Msg {
		if len(directories) == 0 {
			return errMsg{err: fmt.Errorf("no directories provided")}
		}

		var progressCh chan<- int
		if len(directories) > loadingScreenThreshold {
			progressCh = loadProgressCh
		}

		repos, err := load.SyncLoadWithProgress(directories, progressCh)
		if err != nil {
			return errMsg{err: err}
		}

		return repositoriesLoadedMsg{repos: repos}
	}
}

// listenLoadProgressCmd returns a command that waits for one progress update
// and returns a repoLoadProgressMsg with the new loaded count.
func listenLoadProgressCmd() tea.Cmd {
	return func() tea.Msg {
		n := <-loadProgressCh
		return repoLoadProgressMsg{count: n}
	}
}

// tickCmd returns a command that sends a tick message after a delay.
// Callers outside the jobCompletedMsg handler should use ensureTicking()
// instead to prevent multiple parallel tick chains.
func tickCmd() tea.Cmd {
	return tea.Tick(SpinnerDuration, func(t time.Time) tea.Msg {
		return jobCompletedMsg{}
	})
}

// ensureTicking starts the tick chain if it is not already running.
// It returns nil if a tick is already active, preventing duplicate chains.
func (m *Model) ensureTicking() tea.Cmd {
	if m.tickRunning {
		return nil
	}
	m.tickRunning = true
	return tickCmd()
}

// isLazygitAvailable checks if lazygit is in PATH
func isLazygitAvailable() bool {
	_, err := exec.LookPath("lazygit")
	return err == nil
}
