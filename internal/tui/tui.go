package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Version exposes the application version for use across the TUI.
var Version string

// Run starts the TUI application
func Run(mode string, directories []string) error {
	m := New(mode, directories)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}
