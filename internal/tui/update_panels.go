package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// activatePanel switches to the given side panel (or back to overview for NonePanel).
func (m *Model) activatePanel(panel SidePanelType) {
	m.sidePanel = panel
	if panel == NonePanel {
		return
	}

	repo := m.currentRepository()

	if panel == StatusPanel && repo != nil && !repo.WorkStatus().InFlight() {
		command.RequestExternalRefresh(repo)
	}

	switch panel {
	case BranchPanel:
		items := m.branchPanelItems()
		currentName := ""
		if repo != nil && repo.State != nil && repo.State.Branch != nil {
			currentName = repo.State.Branch.Name
		}
		for i, item := range items {
			if item.Name == currentName {
				m.branchCursor = i
				break
			}
		}
		viewport := m.branchViewportSize(len(items))
		if viewport <= 0 {
			viewport = len(items)
		}
		m.ensureBranchCursorVisible(len(items), viewport)
	case RemotePanel:
		items := m.remotePanelItems()
		currentFull := ""
		if repo != nil && repo.State != nil && repo.State.Branch != nil && repo.State.Branch.Upstream != nil {
			currentFull = repo.State.Branch.Upstream.Name
		}
		for i, item := range items {
			if item.FullName == currentFull {
				m.remoteBranchCursor = i
				break
			}
		}
		viewport := m.remoteViewportSize(len(items))
		if viewport <= 0 {
			viewport = len(items)
		}
		m.ensureRemoteCursorVisible(len(items), viewport)
	}

	m.ensureSelectionWithinBounds(panel)
}

func (m *Model) ensureSelectionWithinBounds(panel SidePanelType) {
	switch panel {
	case BranchPanel:
		length := len(m.branchPanelItems())
		m.branchCursor = clampIndex(m.branchCursor, length)
		viewport := m.branchViewportSize(length)
		if viewport <= 0 {
			viewport = length
		}
		m.ensureBranchCursorVisible(length, viewport)
	case RemotePanel:
		length := len(m.remotePanelItems())
		m.remoteBranchCursor = clampIndex(m.remoteBranchCursor, length)
		viewport := m.remoteViewportSize(length)
		if viewport <= 0 {
			viewport = length
		}
		m.ensureRemoteCursorVisible(length, viewport)
	}
}

func (m *Model) handleBranchPanelKey(key string) (tea.Model, tea.Cmd) {
	items := m.branchPanelItems()
	count := len(items)
	if count == 0 {
		return m, nil
	}
	viewport := m.branchViewportSize(count)
	if viewport <= 0 {
		viewport = count
	}

	var cmd tea.Cmd

	switch key {
	case "up", "k":
		wrapCursor(&m.branchCursor, count, -1)
	case "down", "j":
		wrapCursor(&m.branchCursor, count, 1)
	case "home", "g":
		m.branchCursor = 0
	case "end", "G":
		m.branchCursor = count - 1
	case " ", "space", "c":
		branchName := items[clampIndex(m.branchCursor, count)].Name
		if branchName == "" || branchName == "<unknown>" {
			break
		}
		if m.hasMultipleTagged() {
			m.ensureBranchCursorVisible(count, viewport)
			return m, m.checkoutBranchMultiCmd(m.taggedRepositories(), branchName)
		}
		repos := m.panelRepositories()
		if len(repos) == 0 {
			break
		}
		branch := findBranchByName(repos[0], branchName)
		cmd = m.checkoutBranchCmd(repos[0], branch)
	case "d":
		branchName := items[clampIndex(m.branchCursor, count)].Name
		if branchName == "" || branchName == "<unknown>" {
			break
		}
		if m.hasMultipleTagged() {
			m.ensureBranchCursorVisible(count, viewport)
			return m, m.deleteBranchMultiCmd(m.taggedRepositories(), branchName)
		}
		repos := m.panelRepositories()
		if len(repos) == 0 {
			break
		}
		repo := repos[0]
		if repo.State != nil && repo.State.Branch != nil && repo.State.Branch.Name == branchName {
			repo.State.Message = "cannot delete current branch"
			break
		}
		branch := findBranchByName(repo, branchName)
		cmd = m.deleteBranchCmd(repo, branch)
	}

	m.ensureBranchCursorVisible(count, viewport)
	return m, cmd
}

func (m *Model) handleRemotePanelKey(key string) (tea.Model, tea.Cmd) {
	items := m.remotePanelItems()
	count := len(items)
	if count == 0 {
		return m, nil
	}
	viewport := m.remoteViewportSize(count)
	if viewport <= 0 {
		viewport = count
	}

	var cmd tea.Cmd

	switch key {
	case "up", "k":
		wrapCursor(&m.remoteBranchCursor, count, -1)
	case "down", "j":
		wrapCursor(&m.remoteBranchCursor, count, 1)
	case "home", "g":
		m.remoteBranchCursor = 0
	case "end", "G":
		m.remoteBranchCursor = count - 1
	case " ", "space", "c":
		entry := items[clampIndex(m.remoteBranchCursor, count)]
		if m.hasMultipleTagged() {
			m.ensureRemoteCursorVisible(count, viewport)
			return m, m.checkoutRemoteBranchMultiCmd(m.taggedRepositories(), entry)
		}
		repos := m.panelRepositories()
		if len(repos) > 0 {
			cmd = m.checkoutRemoteBranchCmd(repos[0], entry)
		}
	case "d":
		entry := items[clampIndex(m.remoteBranchCursor, count)]
		if m.hasMultipleTagged() {
			m.ensureRemoteCursorVisible(count, viewport)
			return m, m.deleteRemoteBranchMultiCmd(m.taggedRepositories(), entry)
		}
		repos := m.panelRepositories()
		if len(repos) > 0 {
			cmd = m.deleteRemoteBranchCmd(repos[0], entry)
		}
	}

	m.ensureRemoteCursorVisible(count, viewport)
	return m, cmd
}


// --- Git operation commands ---

func (m *Model) checkoutBranchCmd(repo *git.Repository, branch *git.Branch) tea.Cmd {
	if repo == nil || branch == nil {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("checking out %s", branch.Name)
		if err := repo.Checkout(branch); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("checkout branch %s: %w", branch.Name, err)}
		}
		repo.State.Message = fmt.Sprintf("switched to %s", branch.Name)
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return repoActionResultMsg{panel: BranchPanel}
	}
}

func (m *Model) deleteBranchCmd(repo *git.Repository, branch *git.Branch) tea.Cmd {
	if repo == nil || branch == nil {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("deleting %s", branch.Name)
		if _, err := command.Run(repo.AbsPath, "git", []string{"branch", "-d", branch.Name}); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("delete branch %s: %w", branch.Name, err)}
		}
		repo.State.Message = fmt.Sprintf("deleted %s", branch.Name)
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return repoActionResultMsg{panel: BranchPanel}
	}
}

func (m *Model) checkoutBranchMultiCmd(repos []*git.Repository, branchName string) tea.Cmd {
	filtered := filterRepositories(repos)
	if len(filtered) == 0 || branchName == "" {
		return nil
	}
	return func() tea.Msg {
		branchLookup := make(map[*git.Repository]*git.Branch, len(filtered))
		for _, repo := range filtered {
			branch := findBranchByName(repo, branchName)
			if branch == nil {
				repo.State.Message = fmt.Sprintf("branch %s not found", branchName)
				return repoActionResultMsg{panel: BranchPanel}
			}
			branchLookup[repo] = branch
		}
		for _, repo := range filtered {
			repo.State.Message = fmt.Sprintf("checking out %s", branchName)
			if err := repo.Checkout(branchLookup[repo]); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("checkout branch %s in %s: %w", branchName, repo.Name, err)}
			}
			repo.State.Message = fmt.Sprintf("switched to %s", branchName)
			if err := scheduleRefresh(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: BranchPanel, closePanel: true}
	}
}

func (m *Model) deleteBranchMultiCmd(repos []*git.Repository, branchName string) tea.Cmd {
	filtered := filterRepositories(repos)
	if len(filtered) == 0 || branchName == "" {
		return nil
	}
	return func() tea.Msg {
		for _, repo := range filtered {
			if repo.State != nil && repo.State.Branch != nil && repo.State.Branch.Name == branchName {
				repo.State.Message = fmt.Sprintf("cannot delete current branch in %s", repo.Name)
				return repoActionResultMsg{panel: BranchPanel}
			}
		}
		for _, repo := range filtered {
			repo.State.Message = fmt.Sprintf("deleting %s", branchName)
			if _, err := command.Run(repo.AbsPath, "git", []string{"branch", "-d", branchName}); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("delete branch %s in %s: %w", branchName, repo.Name, err)}
			}
			repo.State.Message = fmt.Sprintf("deleted %s", branchName)
			if err := scheduleRefresh(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: BranchPanel}
	}
}

func (m *Model) checkoutRemoteBranchCmd(repo *git.Repository, entry remotePanelEntry) tea.Cmd {
	if repo == nil || entry.FullName == "" {
		return nil
	}
	return func() tea.Msg {
		branchName := entry.BranchName
		repo.State.Message = fmt.Sprintf("checking out %s", branchName)
		if existing := findBranchByName(repo, branchName); existing != nil {
			if err := repo.Checkout(existing); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("checkout branch %s: %w", existing.Name, err)}
			}
		} else {
			args := []string{"checkout", "-b", branchName, entry.FullName}
			if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("create branch %s from %s: %w", branchName, entry.FullName, err)}
			}
		}
		repo.State.Message = fmt.Sprintf("switched to %s", branchName)
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return repoActionResultMsg{panel: RemotePanel}
	}
}

func (m *Model) deleteRemoteBranchCmd(repo *git.Repository, entry remotePanelEntry) tea.Cmd {
	if repo == nil || entry.RemoteName == "" || entry.BranchName == "" {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("deleting %s/%s", entry.RemoteName, entry.BranchName)
		args := []string{"push", entry.RemoteName, "--delete", entry.BranchName}
		if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("delete remote branch %s/%s: %w", entry.RemoteName, entry.BranchName, err)}
		}
		repo.State.Message = fmt.Sprintf("deleted %s/%s", entry.RemoteName, entry.BranchName)
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return repoActionResultMsg{panel: RemotePanel}
	}
}

func (m *Model) checkoutRemoteBranchMultiCmd(repos []*git.Repository, entry remotePanelEntry) tea.Cmd {
	filtered := filterRepositories(repos)
	if len(filtered) == 0 || entry.FullName == "" {
		return nil
	}
	return func() tea.Msg {
		for _, repo := range filtered {
			repo.State.Message = fmt.Sprintf("checking out %s", entry.BranchName)
			if existing := findBranchByName(repo, entry.BranchName); existing != nil {
				if err := repo.Checkout(existing); err != nil {
					repo.State.Message = err.Error()
					return errMsg{err: fmt.Errorf("checkout branch %s in %s: %w", entry.BranchName, repo.Name, err)}
				}
			} else {
				args := []string{"checkout", "-b", entry.BranchName, entry.FullName}
				if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
					repo.State.Message = err.Error()
					return errMsg{err: fmt.Errorf("create branch %s from %s in %s: %w", entry.BranchName, entry.FullName, repo.Name, err)}
				}
			}
			repo.State.Message = fmt.Sprintf("switched to %s", entry.BranchName)
			if err := scheduleRefresh(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: RemotePanel, closePanel: true}
	}
}

func (m *Model) deleteRemoteBranchMultiCmd(repos []*git.Repository, entry remotePanelEntry) tea.Cmd {
	filtered := filterRepositories(repos)
	if len(filtered) == 0 || entry.RemoteName == "" || entry.BranchName == "" {
		return nil
	}
	return func() tea.Msg {
		for _, repo := range filtered {
			repo.State.Message = fmt.Sprintf("deleting %s/%s", entry.RemoteName, entry.BranchName)
			args := []string{"push", entry.RemoteName, "--delete", entry.BranchName}
			if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("delete remote branch %s/%s in %s: %w", entry.RemoteName, entry.BranchName, repo.Name, err)}
			}
			repo.State.Message = fmt.Sprintf("deleted %s/%s", entry.RemoteName, entry.BranchName)
			if err := scheduleRefresh(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: RemotePanel}
	}
}

