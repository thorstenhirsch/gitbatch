package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/job"
)

func (m *Model) handleCredentialPromptKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.activeCredentialPrompt == nil {
		return false, nil
	}

	key := msg.String()
	switch key {
	case "ctrl+c":
		return true, tea.Quit
	case "esc":
		m.cancelCredentialPrompt()
		return true, nil
	case "enter":
		cmd := m.submitCredentialInput()
		return true, cmd
	case "tab", "shift+tab":
		prompt := m.activeCredentialPrompt
		if prompt == nil {
			return true, nil
		}
		switch m.credentialInputField {
		case credentialFieldUsername:
			prompt.username = strings.TrimSpace(m.credentialInputBuffer)
			m.credentialInputField = credentialFieldPassword
			m.credentialInputBuffer = prompt.password
		case credentialFieldPassword:
			prompt.password = m.credentialInputBuffer
			m.credentialInputField = credentialFieldUsername
			m.credentialInputBuffer = prompt.username
		}
		return true, nil
	case "backspace", "ctrl+h":
		m.backspaceCredentialInput()
		return true, nil
	case " ":
		m.credentialInputBuffer += " "
		return true, nil
	default:
		if len(msg.Runes) > 0 {
			m.credentialInputBuffer += string(msg.Runes)
		}
		return true, nil
	}
}

func (m *Model) backspaceCredentialInput() {
	runes := []rune(m.credentialInputBuffer)
	if len(runes) == 0 {
		return
	}
	m.credentialInputBuffer = string(runes[:len(runes)-1])
}

func (m *Model) submitCredentialInput() tea.Cmd {
	prompt := m.activeCredentialPrompt
	if prompt == nil {
		return nil
	}
	switch m.credentialInputField {
	case credentialFieldUsername:
		prompt.username = strings.TrimSpace(m.credentialInputBuffer)
		m.credentialInputField = credentialFieldPassword
		m.credentialInputBuffer = prompt.password
		return nil
	case credentialFieldPassword:
		prompt.password = m.credentialInputBuffer
		cmd := m.retryCredentialPrompt(prompt)
		m.dismissCredentialPrompt()
		return cmd
	default:
		return nil
	}
}

func (m *Model) cancelCredentialPrompt() {
	if m.activeCredentialPrompt != nil && m.activeCredentialPrompt.repo != nil {
		m.activeCredentialPrompt.repo.SetWorkStatus(git.Fail)
		if m.activeCredentialPrompt.repo.State != nil {
			m.activeCredentialPrompt.repo.State.Message = "credentials prompt dismissed"
		}
	}
	m.dismissCredentialPrompt()
}

func (m *Model) dismissCredentialPrompt() {
	m.activeCredentialPrompt = nil
	m.credentialInputBuffer = ""
	m.credentialInputField = credentialFieldUsername
	m.advanceCredentialPrompt()
}

func (m *Model) openCredentialDialog(repo *git.Repository) {
	if repo == nil {
		return
	}

	var existingJob *job.Job
	for _, prompt := range m.credentialPromptQueue {
		if prompt != nil && prompt.repo != nil && prompt.repo.RepoID == repo.RepoID {
			existingJob = prompt.job
			break
		}
	}

	if existingJob == nil {
		existingJob = &job.Job{
			JobType:    job.FetchJob,
			Repository: repo,
			Options: &command.FetchOptions{
				RemoteName: defaultRemoteName(repo),
				Timeout:    command.DefaultFetchTimeout,
			},
		}
	}

	prompt := &credentialPrompt{
		repo: repo,
		job:  existingJob,
	}

	if m.activeCredentialPrompt == nil {
		m.activeCredentialPrompt = prompt
		m.credentialInputField = credentialFieldUsername
		m.credentialInputBuffer = ""
	} else {
		m.credentialPromptQueue = append(m.credentialPromptQueue, prompt)
	}
}

func (m *Model) advanceCredentialPrompt() {
	if len(m.credentialPromptQueue) == 0 {
		m.activeCredentialPrompt = nil
		m.credentialInputBuffer = ""
		m.credentialInputField = credentialFieldUsername
		return
	}
	next := m.credentialPromptQueue[0]
	m.credentialPromptQueue = m.credentialPromptQueue[1:]
	m.activeCredentialPrompt = next
	m.credentialInputField = credentialFieldUsername
	if next != nil {
		m.credentialInputBuffer = next.username
	} else {
		m.credentialInputBuffer = ""
	}
}

func (m *Model) retryCredentialPrompt(prompt *credentialPrompt) tea.Cmd {
	if prompt == nil || prompt.repo == nil || prompt.job == nil {
		return nil
	}
	repo := prompt.repo
	repo.SetWorkStatus(git.Pending)
	if repo.State != nil {
		repo.State.Message = "retrying with credentials"
	}
	creds := &git.Credentials{
		User:     strings.TrimSpace(prompt.username),
		Password: prompt.password,
	}
	retryJob := cloneJobWithCredentials(prompt.job, creds)
	if retryJob == nil {
		repo.SetWorkStatus(git.Fail)
		if repo.State != nil {
			repo.State.Message = "unable to retry with credentials"
		}
		return nil
	}
	retryJob.Repository = repo
	if err := retryJob.Start(); err != nil {
		repo.SetWorkStatus(git.Fail)
		if repo.State != nil {
			repo.State.Message = "failed to start credential retry"
		}
		return func() tea.Msg { return errMsg{err: err} }
	}
	repo.SetWorkStatus(git.Queued)
	m.jobsRunning = true
	return tickCmd()
}

// cloneJobWithCredentials duplicates a job and injects the provided credentials.
func cloneJobWithCredentials(original *job.Job, creds *git.Credentials) *job.Job {
	if original == nil {
		return nil
	}
	clone := &job.Job{
		JobType:    original.JobType,
		Repository: original.Repository,
	}

	switch original.JobType {
	case job.FetchJob:
		var opts *command.FetchOptions
		switch cfg := original.Options.(type) {
		case *command.FetchOptions:
			copyCfg := *cfg
			copyCfg.Credentials = creds
			if copyCfg.Timeout <= 0 {
				copyCfg.Timeout = command.DefaultFetchTimeout
			}
			opts = &copyCfg
		case command.FetchOptions:
			copyCfg := cfg
			copyCfg.Credentials = creds
			if copyCfg.Timeout <= 0 {
				copyCfg.Timeout = command.DefaultFetchTimeout
			}
			opts = &copyCfg
		default:
			opts = &command.FetchOptions{
				RemoteName:  defaultRemoteName(original.Repository),
				Timeout:     command.DefaultFetchTimeout,
				Credentials: creds,
			}
		}
		clone.Options = opts
	case job.PullJob, job.RebaseJob:
		switch cfg := original.Options.(type) {
		case *command.PullOptions:
			copyCfg := *cfg
			copyCfg.Credentials = creds
			clone.Options = &copyCfg
		case command.PullOptions:
			copyCfg := cfg
			copyCfg.Credentials = creds
			clone.Options = &copyCfg
		case *job.PullJobConfig:
			copyCfg := *cfg
			if copyCfg.Options != nil {
				optsCopy := *copyCfg.Options
				optsCopy.Credentials = creds
				copyCfg.Options = &optsCopy
			} else {
				copyCfg.Options = &command.PullOptions{Credentials: creds}
			}
			clone.Options = &copyCfg
		case job.PullJobConfig:
			copyCfg := cfg
			if copyCfg.Options != nil {
				optsCopy := *copyCfg.Options
				optsCopy.Credentials = creds
				copyCfg.Options = &optsCopy
			} else {
				copyCfg.Options = &command.PullOptions{Credentials: creds}
			}
			clone.Options = &copyCfg
		default:
			clone.Options = &command.PullOptions{Credentials: creds}
		}
	default:
		return nil
	}

	return clone
}
