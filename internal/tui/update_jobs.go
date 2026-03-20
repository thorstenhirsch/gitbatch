package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/job"
)

// toggleQueue adds or removes the selected repository from the queue.
func (m *Model) toggleQueue() tea.Cmd {
	if len(m.repositories) == 0 {
		return nil
	}
	r := m.repositories[m.cursor]
	if r == nil {
		return nil
	}
	if r.WorkStatus() == git.Queued {
		return func() tea.Msg {
			m.removeFromQueue(r)
			return jobCompletedMsg{}
		}
	}
	if !repoIsActionable(r) {
		return nil
	}
	return func() tea.Msg {
		m.addToQueue(r)
		return jobCompletedMsg{}
	}
}

// addToQueue marks a repository as queued for later execution.
func (m *Model) addToQueue(r *git.Repository) error {
	if !repoIsActionable(r) {
		return nil
	}
	switch m.mode.ID {
	case PullMode:
		if r.State == nil || r.State.Branch == nil || r.State.Branch.Upstream == nil || r.State.Remote == nil {
			return nil
		}
	case MergeMode:
		if r.State == nil || r.State.Branch == nil || r.State.Branch.Upstream == nil {
			return nil
		}
	case RebaseMode:
		if r.State == nil || r.State.Branch == nil || r.State.Branch.Upstream == nil || r.State.Remote == nil {
			return nil
		}
	case PushMode:
		if r.State == nil || r.State.Remote == nil || r.State.Branch == nil {
			return nil
		}
	default:
		return nil
	}
	r.SetWorkStatusSilent(git.Queued)
	return nil
}

// removeFromQueue removes a repository from the queue.
func (m *Model) removeFromQueue(r *git.Repository) error {
	r.SetWorkStatusSilent(git.Available)
	return nil
}

// queueAll adds all actionable repositories to the queue.
func (m *Model) queueAll() tea.Cmd {
	return func() tea.Msg {
		for _, r := range m.repositories {
			if repoIsActionable(r) {
				m.addToQueue(r)
			}
		}
		return jobCompletedMsg{}
	}
}

// unqueueAll removes all repositories from the queue.
func (m *Model) unqueueAll() tea.Cmd {
	return func() tea.Msg {
		for _, r := range m.repositories {
			if r.WorkStatus() == git.Queued {
				m.removeFromQueue(r)
			}
		}
		return jobCompletedMsg{}
	}
}

// startQueue starts jobs for all queued repositories.
func (m *Model) startQueue() tea.Cmd {
	return func() tea.Msg {
		for _, r := range m.repositories {
			if r.WorkStatus() != git.Queued {
				continue
			}
			j := &job.Job{Repository: r}

			switch m.mode.ID {
			case PullMode:
				if r.State == nil || r.State.Branch == nil || r.State.Branch.Upstream == nil || r.State.Remote == nil {
					continue
				}
				j.JobType = job.PullJob
				j.Options = &command.PullOptions{RemoteName: r.State.Remote.Name, FFOnly: true}
			case MergeMode:
				if r.State == nil || r.State.Branch == nil || r.State.Branch.Upstream == nil {
					continue
				}
				j.JobType = job.MergeJob
			case RebaseMode:
				if r.State == nil || r.State.Branch == nil || r.State.Branch.Upstream == nil || r.State.Remote == nil {
					continue
				}
				j.JobType = job.RebaseJob
				j.Options = &command.PullOptions{RemoteName: r.State.Remote.Name, Rebase: true}
			case PushMode:
				if r.State == nil || r.State.Remote == nil || r.State.Branch == nil {
					continue
				}
				j.JobType = job.PushJob
				j.Options = &command.PushOptions{RemoteName: r.State.Remote.Name, ReferenceName: r.State.Branch.Name}
			default:
				continue
			}

			if err := j.Start(); err != nil {
				r.SetWorkStatus(git.Available)
				r.State.Message = fmt.Sprintf("failed to start: %v", err)
			}
		}
		m.jobsRunning = true
		return jobCompletedMsg{}
	}
}

func (m *Model) runFetchForRepo(repo *git.Repository) tea.Cmd {
	if repo == nil || !repoIsActionable(repo) || repoHasActiveJob(repo.WorkStatus()) {
		return nil
	}
	return m.startFetchForRepos([]*git.Repository{repo})
}

func (m *Model) runPullForRepo(repo *git.Repository, suppressSuccess bool) tea.Cmd {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return nil
	}
	if !repoIsActionable(repo) || repoHasActiveJob(repo.WorkStatus()) {
		return nil
	}
	if repo.State.Branch.Upstream == nil {
		repo.State.Message = "upstream not set"
		return nil
	}
	if repo.State.Remote == nil {
		repo.State.Message = "remote not set"
		return nil
	}
	repo.State.Message = "pull queued"
	repo.SetWorkStatus(git.Pending)
	j := &job.Job{
		Repository: repo,
		JobType:    job.PullJob,
		Options: &job.PullJobConfig{
			Options:         &command.PullOptions{RemoteName: repo.State.Remote.Name, FFOnly: true},
			SuppressSuccess: suppressSuccess,
		},
	}
	if err := j.Start(); err != nil {
		repo.SetWorkStatus(git.Available)
		return func() tea.Msg { return errMsg{err: err} }
	}
	repo.SetWorkStatus(git.Queued)
	m.jobsRunning = true
	return tickCmd()
}

func (m *Model) runPushForRepo(repo *git.Repository, force bool, suppressSuccess bool, message string) tea.Cmd {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return nil
	}
	if repoHasActiveJob(repo.WorkStatus()) || !repoIsActionable(repo) {
		return nil
	}
	if repo.State.Remote == nil {
		repo.State.Message = "remote not set"
		return nil
	}
	if repo.State.Branch.Name == "" {
		repo.State.Message = "branch not set"
		return nil
	}
	if message == "" {
		if force {
			message = "force push queued"
		} else {
			message = "push queued"
		}
	}
	repo.State.Message = message
	repo.SetWorkStatus(git.Pending)
	j := &job.Job{
		Repository: repo,
		JobType:    job.PushJob,
		Options: &job.PushJobConfig{
			Options: &command.PushOptions{
				RemoteName:    repo.State.Remote.Name,
				ReferenceName: repo.State.Branch.Name,
				Force:         force,
			},
			SuppressSuccess: suppressSuccess,
		},
	}
	if err := j.Start(); err != nil {
		repo.SetWorkStatus(git.Available)
		return func() tea.Msg { return errMsg{err: err} }
	}
	repo.SetWorkStatus(git.Queued)
	m.jobsRunning = true
	return tickCmd()
}

// advanceForcePrompt moves to the next queued force-push prompt.
func (m *Model) advanceForcePrompt() {
	if len(m.forcePromptQueue) == 0 {
		m.activeForcePrompt = nil
		return
	}
	m.activeForcePrompt = m.forcePromptQueue[0]
	m.forcePromptQueue = m.forcePromptQueue[1:]
}

func (m *Model) dismissForcePrompt() {
	m.activeForcePrompt = nil
	m.advanceForcePrompt()
}

func (m *Model) confirmForcePush() tea.Cmd {
	if m.activeForcePrompt == nil {
		return nil
	}
	repo := m.activeForcePrompt.repo
	m.dismissForcePrompt()
	if repo == nil {
		return nil
	}
	return m.runPushForRepo(repo, true, true, "retrying push with --force")
}

func (m *Model) startFetchForRepos(repos []*git.Repository) tea.Cmd {
	if len(repos) == 0 {
		return nil
	}
	eligible := make([]*git.Repository, 0, len(repos))
	seen := make(map[string]struct{}, len(repos))
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		if _, ok := seen[repo.RepoID]; ok {
			continue
		}
		seen[repo.RepoID] = struct{}{}
		if !repoIsActionable(repo) {
			continue
		}
		status := repo.WorkStatus()
		if status == git.Working || status == git.Queued || status == git.Pending {
			continue
		}
		if repo.State == nil || repo.State.Remote == nil {
			if repo.State != nil && repo.State.Message == "" {
				repo.State.Message = "no remote configured"
			}
			continue
		}
		repo.State.Message = ""
		repo.SetWorkStatus(git.Pending)
		eligible = append(eligible, repo)
	}
	if len(eligible) == 0 {
		return nil
	}
	m.jobsRunning = true
	return tea.Batch(fetchRepositoriesCmd(eligible), tickCmd())
}

func fetchRepositoriesCmd(repos []*git.Repository) tea.Cmd {
	return func() tea.Msg {
		for _, repo := range repos {
			opts := &command.FetchOptions{
				RemoteName: defaultRemoteName(repo),
				Timeout:    command.DefaultFetchTimeout,
			}
			j := &job.Job{JobType: job.FetchJob, Repository: repo, Options: opts}
			if err := j.Start(); err != nil {
				command.ScheduleStateEvaluation(repo, command.OperationOutcome{
					Operation: command.OperationFetch,
					Err:       err,
					Message:   err.Error(),
				})
			}
			// Yield briefly so the git queue doesn't saturate before the TUI can render.
			time.Sleep(10 * time.Millisecond)
		}
		return jobCompletedMsg{}
	}
}

func (m *Model) maybeStartInitialStateEvaluation(repos []*git.Repository) tea.Cmd {
	if m.initialStateProbeStarted || m.loading || m.terminalTooSmall() {
		return nil
	}
	reposToUse := repos
	if reposToUse == nil {
		reposToUse = m.repositories
	}
	if len(reposToUse) == 0 {
		return nil
	}
	filtered := filterRepositories(reposToUse)
	return func() tea.Msg {
		for _, repo := range filtered {
			if repo == nil {
				continue
			}
			if repo.State != nil {
				repo.State.Message = "waiting"
			}
			repo.SetWorkStatusSilent(git.Pending)
			command.ScheduleStateEvaluation(repo, command.OperationOutcome{Operation: command.OperationStateProbe})
		}
		m.initialStateProbeStarted = true
		m.jobsRunning = true
		return repositoriesWaitingMsg{}
	}
}
