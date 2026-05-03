package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func (m *Model) handleWorktreePromptKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.worktreePromptActive {
		return false, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return true, tea.Quit
	case "esc":
		m.dismissWorktreePrompt()
		return true, nil
	case "tab":
		m.switchWorktreeField()
		return true, nil
	case "enter":
		if m.worktreePromptField == worktreeFieldBranch {
			m.worktreePromptField = worktreeFieldPath
			return true, nil
		}
		return true, m.submitWorktreePrompt()
	case "backspace", "ctrl+h":
		m.backspaceWorktreeInput()
		return true, nil
	case " ":
		if m.worktreePromptField == worktreeFieldBranch {
			m.worktreeBranchBuffer += " "
		} else {
			m.worktreePathBuffer += " "
		}
		return true, nil
	default:
		if len(msg.Runes) > 0 {
			if m.worktreePromptField == worktreeFieldBranch {
				m.worktreeBranchBuffer += string(msg.Runes)
			} else {
				m.worktreePathBuffer += string(msg.Runes)
			}
		}
		return true, nil
	}
}

func (m *Model) openWorktreePrompt() {
	if m.hasMultipleTagged() {
		m.notifyMultiSelectionRestriction("worktree creation is only available for one repository")
		return
	}
	repo := m.currentWorktreeCommandRepository()
	if repo == nil {
		return
	}
	m.worktreePromptActive = true
	m.worktreePromptRepo = repo
	m.worktreePromptField = worktreeFieldBranch
	m.worktreeBranchBuffer = ""
	m.worktreePathBuffer = ""
}

func (m *Model) dismissWorktreePrompt() {
	m.worktreePromptActive = false
	m.worktreePromptRepo = nil
	m.worktreePromptField = worktreeFieldBranch
	m.worktreeBranchBuffer = ""
	m.worktreePathBuffer = ""
}

func (m *Model) switchWorktreeField() {
	if m.worktreePromptField == worktreeFieldBranch {
		m.worktreePromptField = worktreeFieldPath
		return
	}
	m.worktreePromptField = worktreeFieldBranch
}

func (m *Model) backspaceWorktreeInput() {
	if m.worktreePromptField == worktreeFieldBranch {
		runes := []rune(m.worktreeBranchBuffer)
		if len(runes) > 0 {
			m.worktreeBranchBuffer = string(runes[:len(runes)-1])
		}
		return
	}
	runes := []rune(m.worktreePathBuffer)
	if len(runes) > 0 {
		m.worktreePathBuffer = string(runes[:len(runes)-1])
	}
}

func (m *Model) submitWorktreePrompt() tea.Cmd {
	repo := m.worktreePromptRepo
	branchName := strings.TrimSpace(m.worktreeBranchBuffer)
	path := strings.TrimSpace(m.worktreePathBuffer)
	m.dismissWorktreePrompt()

	if repo == nil {
		return nil
	}
	if branchName == "" {
		repo.State.Message = "worktree branch name required"
		return nil
	}
	if path == "" {
		repo.State.Message = "worktree path required"
		return nil
	}
	return m.createWorktreeCmd(repo, branchName, path)
}

func (m *Model) createWorktreeCmd(repo *git.Repository, branchName, path string) tea.Cmd {
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("creating worktree %s", branchName)
		newBranch := !repo.LocalBranchExists(branchName)
		if err := repo.CreateWorktree(git.WorktreeAddOptions{
			Path:       path,
			BranchName: branchName,
			NewBranch:  newBranch,
		}); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("create worktree %s: %w", branchName, err)}
		}
		repo.State.Message = fmt.Sprintf("created worktree %s", branchName)
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return jobCompletedMsg{}
	}
}

func (m *Model) deleteSelectedWorktreeCmd() tea.Cmd {
	row, ok := m.currentOverviewRow()
	if !ok || row.kind != overviewWorktreeRow || row.worktree == nil {
		return nil
	}
	repo := row.actionRepository()
	if repo == nil {
		return nil
	}
	worktree := row.worktree
	if worktree.IsPrimary {
		if repo.State != nil {
			repo.State.Message = "cannot delete [main] worktree"
		}
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("deleting worktree %s", worktree.DisplayName())
		if err := repo.RemoveWorktree(worktree, false); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("delete worktree %s: %w", worktree.DisplayName(), err)}
		}
		m.removeRepositoryByPath(worktree.Path)
		repo.State.Message = fmt.Sprintf("deleted worktree %s", worktree.DisplayName())
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		m.cursor = m.closestSelectableIndex(m.cursor, -1)
		return jobCompletedMsg{}
	}
}

func (m *Model) pruneWorktreesCmd() tea.Cmd {
	row, ok := m.currentOverviewRow()
	if !ok {
		return nil
	}
	repo := row.actionRepository()
	if repo == nil {
		repo = row.repository()
	}
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = "pruning stale worktrees"
		if err := repo.PruneWorktrees(); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("worktree prune: %w", err)}
		}
		repo.State.Message = "pruned stale worktrees"
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return jobCompletedMsg{}
	}
}

func (m *Model) toggleWorktreeLockCmd() tea.Cmd {
	row, ok := m.currentOverviewRow()
	if !ok || row.kind != overviewWorktreeRow || row.worktree == nil {
		return nil
	}
	if row.worktree.IsPrimary {
		return nil
	}
	repo := row.actionRepository()
	if repo == nil {
		return nil
	}
	worktree := row.worktree
	return func() tea.Msg {
		var err error
		if worktree.IsLocked {
			repo.State.Message = fmt.Sprintf("unlocking worktree %s", worktree.DisplayName())
			err = repo.UnlockWorktree(worktree)
			if err == nil {
				repo.State.Message = fmt.Sprintf("unlocked worktree %s", worktree.DisplayName())
			}
		} else {
			repo.State.Message = fmt.Sprintf("locking worktree %s", worktree.DisplayName())
			err = repo.LockWorktree(worktree, "")
			if err == nil {
				repo.State.Message = fmt.Sprintf("locked worktree %s", worktree.DisplayName())
			}
		}
		if err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("worktree lock toggle %s: %w", worktree.DisplayName(), err)}
		}
		if err := scheduleRefresh(repo); err != nil {
			return errMsg{err: err}
		}
		return jobCompletedMsg{}
	}
}

func (m *Model) removeRepositoryByPath(path string) {
	normalized := normalizeOverviewPath(path)
	filtered := m.repositories[:0]
	for _, repo := range m.repositories {
		if repo == nil || normalizeOverviewPath(repo.AbsPath) == normalized {
			continue
		}
		filtered = append(filtered, repo)
	}
	m.repositories = filtered
}
