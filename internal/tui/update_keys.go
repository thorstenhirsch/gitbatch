package tui

import (
	"os/exec"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.commitPromptActive {
		handled, cmd := m.handleCommitPromptKey(msg)
		if handled {
			return m, cmd
		}
	}

	if m.stashPromptActive {
		handled, cmd := m.handleStashPromptKey(msg)
		if handled {
			return m, cmd
		}
	}

	if m.activeCredentialPrompt != nil {
		handled, cmd := m.handleCredentialPromptKey(msg)
		if handled {
			return m, cmd
		}
	}

	if m.activeForcePrompt != nil {
		switch key {
		case "y", "Y", "enter":
			return m, m.confirmForcePush()
		case "n", "N", "esc":
			m.dismissForcePrompt()
			return m, nil
		default:
			return m, nil
		}
	}

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "?":
		m.showHelp = !m.showHelp
		return m, nil

	case "esc":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.sidePanel != NonePanel {
			m.sidePanel = NonePanel
			m.clearSuccessFormatting()
			return m, nil
		}
		m.clearSuccessFormatting()
		return m, nil

	case "tab":
		if len(m.repositories) > 0 && m.cursor < len(m.repositories) {
			r := m.repositories[m.cursor]
			if isLazygitAvailable() {
				var savedState git.RepositoryState
				if r.State != nil {
					savedState = *r.State
				}
				r.SetWorkStatus(git.Working)
				r.RefreshModTime()
				originalModTime := r.ModTime
				cmd := tea.ExecProcess(exec.Command("lazygit", "-p", r.AbsPath), func(err error) tea.Msg {
					if err != nil {
						return errMsg{err: err}
					}
					return lazygitClosedMsg{repo: r, originalModTime: originalModTime, originalState: savedState}
				})
				if m.updateJobsRunningFlag() {
					return m, tea.Batch(cmd, tickCmd())
				}
				return m, cmd
			}
		}
		return m, nil
	}

	if m.sidePanel != NonePanel {
		return m.handleFocusKeys(msg)
	}
	return m.handleOverviewKeys(msg)
}

func (m *Model) handleOverviewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if len(m.repositories) == 0 {
			return m, nil
		}
		m.cursor = (m.cursor - 1 + len(m.repositories)) % len(m.repositories)
		m.cursor = m.findNextReadyIndex(m.cursor)
		m.resetCommitScrollForSelected()
		return m, nil

	case "down", "j":
		if len(m.repositories) == 0 {
			return m, nil
		}
		m.cursor = (m.cursor + 1) % len(m.repositories)
		m.cursor = m.findNextReadyIndex(m.cursor)
		m.resetCommitScrollForSelected()
		return m, nil

	case "g":
		m.cursor = 0
		m.resetCommitScrollForSelected()

	case "G":
		m.cursor = m.findLastNavigableIndex()
		m.resetCommitScrollForSelected()

	case "home":
		m.cursor = 0
		m.resetCommitScrollForSelected()

	case "end":
		m.cursor = m.findLastNavigableIndex()
		m.resetCommitScrollForSelected()

	case "ctrl+f", "pgdown":
		pageSize := m.height - 5
		m.cursor += pageSize
		if m.cursor >= len(m.repositories) {
			m.cursor = len(m.repositories) - 1
		}
		m.cursor = m.findNextReadyIndex(m.cursor)
		m.resetCommitScrollForSelected()

	case "ctrl+b", "pgup":
		pageSize := m.height - 5
		m.cursor -= pageSize
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.cursor = m.findNextReadyIndex(m.cursor)
		m.resetCommitScrollForSelected()

	case "ctrl+d":
		halfPage := (m.height - 5) / 2
		m.cursor += halfPage
		if m.cursor >= len(m.repositories) {
			m.cursor = len(m.repositories) - 1
		}
		m.cursor = m.findNextReadyIndex(m.cursor)
		m.resetCommitScrollForSelected()

	case "ctrl+u":
		halfPage := (m.height - 5) / 2
		m.cursor -= halfPage
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.cursor = m.findNextReadyIndex(m.cursor)
		m.resetCommitScrollForSelected()

	case "right", "l":
		if m.adjustCommitScroll(12) {
			return m, nil
		}

	case "left", "h":
		if m.adjustCommitScroll(-12) {
			return m, nil
		}

	case " ", "space":
		return m, m.toggleQueue()

	case "a":
		return m, m.queueAll()

	case "A":
		return m, m.unqueueAll()

	case "enter":
		repo := m.currentRepository()
		if repo != nil && repo.State != nil && repo.State.RequiresCredentials {
			ws := repo.WorkStatus()
			if ws != git.Working && ws != git.Queued && ws != git.Pending {
				m.openCredentialDialog(repo)
				return m, nil
			}
		}
		return m, m.startQueue()

	case "f":
		repo := m.currentRepository()
		if repo == nil || !repoIsActionable(repo) {
			return m, nil
		}
		return m, m.runFetchForRepo(repo)

	case "p":
		repo := m.currentRepository()
		if repo == nil || !repoIsActionable(repo) {
			return m, nil
		}
		return m, m.runPullForRepo(repo, true)

	case "P":
		repo := m.currentRepository()
		if repo == nil || !repoIsActionable(repo) {
			return m, nil
		}
		return m, m.runPushForRepo(repo, false, true, "push queued")

	case "m":
		m.cycleMode()

	case "b":
		m.activatePanel(BranchPanel)

	case "B":
		m.expandBranches = !m.expandBranches

	case "c":
		if m.err != nil {
			m.err = nil
			return m, nil
		}
		if len(m.repositories) > 0 && m.cursor < len(m.repositories) {
			repo := m.repositories[m.cursor]
			if repo != nil && repo.State != nil && repo.WorkStatus() == git.Fail {
				repo.State.Message = ""
				return m, nil
			}
		}
		m.openCommitPrompt()
		return m, nil

	case "r":
		m.activatePanel(RemotePanel)

	case "s":
		if !m.requiresSingleSelection("Status view unavailable for tagged selection") {
			return m, nil
		}
		m.activatePanel(StatusPanel)

	case "S":
		m.openStashPrompt()
		return m, nil

	case "O":
		m.openStashAction(stashActionPop)
		return m, nil

	case "D":
		m.openStashAction(stashActionDrop)
		return m, nil

	case "n":
		m.sortByName()

	case "t":
		m.sortByTime()
	}

	return m, nil
}

func (m *Model) handleFocusKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "backspace":
		m.sidePanel = NonePanel
		return m, nil
	}
	switch m.sidePanel {
	case BranchPanel:
		return m.handleBranchPanelKey(key)
	case RemotePanel:
		return m.handleRemotePanelKey(key)
	case StashActionPanel:
		return m.handleStashActionPanelKey(key)
	default:
		return m, nil
	}
}

func (m *Model) cycleMode() {
	for i, mode := range modes {
		if mode.ID == m.mode.ID {
			m.mode = modes[(i+1)%len(modes)]
			return
		}
	}
}

func (m *Model) sortByName() {
	sort.Sort(git.Alphabetical(m.repositories))
}

func (m *Model) sortByTime() {
	sort.Sort(git.LastModified(m.repositories))
}

func (m *Model) clearSuccessFormatting() {
	for _, repo := range m.repositories {
		if repo != nil && repo.WorkStatus() == git.Success {
			repo.SetWorkStatus(git.Available)
		}
	}
}

// requiresSingleSelection returns true if the action can proceed (no multi-selection active).
// When false it has already notified the user via message.
func (m *Model) requiresSingleSelection(message string) bool {
	if !m.hasMultipleTagged() {
		return true
	}
	m.notifyMultiSelectionRestriction(message)
	return false
}

func (m *Model) notifyMultiSelectionRestriction(message string) {
	repos := m.taggedRepositories()
	if len(repos) == 0 {
		repo := m.currentRepository()
		if repo != nil && repo.State != nil {
			repo.State.Message = message
		}
		return
	}
	for _, repo := range repos {
		if repo != nil && repo.State != nil {
			repo.State.Message = message
		}
	}
}

