package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/job"
)

func (m *Model) handleCommitPromptKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.commitPromptActive {
		return false, nil
	}

	key := msg.String()
	switch key {
	case "ctrl+c":
		return true, tea.Quit
	case "esc":
		m.dismissCommitPrompt()
		return true, nil
	case "tab":
		m.switchCommitField()
		return true, nil
	case "enter":
		if m.commitPromptField == commitFieldMessage {
			cmd := m.submitCommit()
			return true, cmd
		}
		// In description field, enter inserts a newline
		m.commitDescBuffer += "\n"
		return true, nil
	case "backspace", "ctrl+h":
		m.backspaceCommitInput()
		return true, nil
	case " ":
		if m.commitPromptField == commitFieldMessage {
			m.commitMessageBuffer += " "
		} else {
			m.commitDescBuffer += " "
		}
		return true, nil
	default:
		if len(msg.Runes) > 0 {
			if m.commitPromptField == commitFieldMessage {
				m.commitMessageBuffer += string(msg.Runes)
			} else {
				m.commitDescBuffer += string(msg.Runes)
			}
		}
		return true, nil
	}
}

func (m *Model) switchCommitField() {
	if m.commitPromptField == commitFieldMessage {
		m.commitPromptField = commitFieldDescription
	} else {
		m.commitPromptField = commitFieldMessage
	}
}

func (m *Model) backspaceCommitInput() {
	if m.commitPromptField == commitFieldMessage {
		runes := []rune(m.commitMessageBuffer)
		if len(runes) > 0 {
			m.commitMessageBuffer = string(runes[:len(runes)-1])
		}
	} else {
		runes := []rune(m.commitDescBuffer)
		if len(runes) > 0 {
			m.commitDescBuffer = string(runes[:len(runes)-1])
		}
	}
}

func (m *Model) openCommitPrompt() {
	repos := m.taggedRepositories()
	if len(repos) == 0 {
		repo := m.currentRepository()
		if repo == nil {
			return
		}
		repos = []*git.Repository{repo}
	}
	// Filter to only repos with local changes
	var eligible []*git.Repository
	for _, repo := range repos {
		if repoHasLocalChanges(repo) {
			eligible = append(eligible, repo)
		}
	}
	if len(eligible) == 0 {
		return
	}
	m.commitPromptActive = true
	m.commitPromptRepos = eligible
	m.commitPromptField = commitFieldMessage
	m.commitMessageBuffer = ""
	m.commitDescBuffer = ""
}

func (m *Model) dismissCommitPrompt() {
	m.commitPromptActive = false
	m.commitPromptRepos = nil
	m.commitMessageBuffer = ""
	m.commitDescBuffer = ""
	m.commitPromptField = commitFieldMessage
}

func (m *Model) submitCommit() tea.Cmd {
	msg := strings.TrimSpace(m.commitMessageBuffer)
	if msg == "" {
		return nil
	}
	desc := strings.TrimSpace(m.commitDescBuffer)
	repos := m.commitPromptRepos
	m.dismissCommitPrompt()

	if len(repos) == 0 {
		return nil
	}

	opts := &command.CommitOptions{
		Message:     msg,
		Description: desc,
	}

	m.jobsRunning = true
	return func() tea.Msg {
		for _, repo := range repos {
			if repo == nil || repo.State == nil {
				continue
			}
			repo.State.Message = "committing.."
			repo.SetWorkStatus(git.Pending)
			j := &job.Job{
				Repository: repo,
				JobType:    job.CommitJob,
				Options:    opts,
			}
			if err := j.Start(); err != nil {
				repo.SetWorkStatus(git.Available)
				repo.State.Message = "commit failed: " + err.Error()
				continue
			}
		}
		return jobCompletedMsg{}
	}
}
