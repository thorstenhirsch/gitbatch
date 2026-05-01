package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func (m *Model) handleBranchPromptKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.branchPromptActive {
		return false, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return true, tea.Quit
	case "esc":
		m.dismissBranchPrompt()
		return true, nil
	case "enter":
		return true, m.submitBranchPrompt()
	case "backspace", "ctrl+h":
		runes := []rune(m.branchNameBuffer)
		if len(runes) > 0 {
			m.branchNameBuffer = string(runes[:len(runes)-1])
		}
		return true, nil
	case " ":
		m.branchNameBuffer += " "
		return true, nil
	default:
		if len(msg.Runes) > 0 {
			m.branchNameBuffer += string(msg.Runes)
		}
		return true, nil
	}
}

func (m *Model) openBranchPrompt() {
	repos := filterRepositories(m.taggedRepositories())
	if len(repos) == 0 {
		if repo := m.currentRepository(); repo != nil {
			repos = []*git.Repository{repo}
		}
	}
	if len(repos) == 0 {
		return
	}

	m.branchPromptActive = true
	m.branchPromptRepos = repos
	m.branchNameBuffer = ""
}

func (m *Model) dismissBranchPrompt() {
	m.branchPromptActive = false
	m.branchPromptRepos = nil
	m.branchNameBuffer = ""
}

func (m *Model) submitBranchPrompt() tea.Cmd {
	repos := m.branchPromptRepos
	branchName := strings.TrimSpace(m.branchNameBuffer)
	m.dismissBranchPrompt()

	if len(repos) == 0 {
		return nil
	}
	if branchName == "" {
		for _, repo := range repos {
			if repo != nil && repo.State != nil {
				repo.State.Message = "branch name required"
			}
		}
		return nil
	}
	return m.createBranchCmd(repos, branchName)
}

func (m *Model) createBranchCmd(repos []*git.Repository, branchName string) tea.Cmd {
	filtered := filterRepositories(repos)
	if len(filtered) == 0 || branchName == "" {
		return nil
	}

	return func() tea.Msg {
		for _, repo := range filtered {
			if findBranchByName(repo, branchName) != nil {
				if repo.State != nil {
					repo.State.Message = fmt.Sprintf("branch %s already exists", branchName)
				}
				return repoActionResultMsg{panel: BranchPanel}
			}
		}

		for _, repo := range filtered {
			if repo.State != nil {
				repo.State.Message = fmt.Sprintf("creating %s", branchName)
			}
			if _, err := command.Run(repo.AbsPath, "git", []string{"checkout", "-b", branchName}); err != nil {
				if repo.State != nil {
					repo.State.Message = err.Error()
				}
				return errMsg{err: fmt.Errorf("create branch %s in %s: %w", branchName, repo.Name, err)}
			}
			if repo.State != nil {
				repo.State.Message = fmt.Sprintf("switched to %s", branchName)
			}
			if err := scheduleRefresh(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}

		return repoActionResultMsg{panel: BranchPanel}
	}
}
