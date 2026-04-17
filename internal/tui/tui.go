package tui

import (
	"io"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/watch"
)

// Version exposes the application version for use across the TUI.
var Version string

// Run starts the TUI application
func Run(mode string, directories []string) error {
	// The standard logger writes to stderr, which shares the terminal with the
	// alt-screen TUI and corrupts the display on every Printf. Route it to
	// /dev/null for the lifetime of the TUI. Trace logging via --trace has its
	// own dedicated file and is unaffected.
	log.SetOutput(io.Discard)

	m := New(mode, directories)
	if svc, err := watch.New(); err == nil {
		m.watcher = svc
		defer svc.Close()
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}
