package tui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/job"
)

// Update handles all messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case repositoriesLoadedMsg:
		repos := make([]*git.Repository, 0, len(msg.repos))
		for _, repo := range msg.repos {
			if repo == nil {
				continue
			}
			repos = append(repos, repo)
			m.addRepository(repo)
		}
		m.loading = false

		if cmd := m.startFetchForRepos(repos); cmd != nil {
			return m, cmd
		}
		return m, nil

	case lazygitClosedMsg:
		// Lazygit has closed, just refresh the display
		return m, nil

	case jobCompletedMsg:
		m.advanceSpinner()
		if m.updateJobsRunningFlag() {
			return m, tickCmd()
		}
		return m, nil

	case jobQueueResultMsg:
		if len(msg.failures) > 0 {
			m.processJobFailures(msg.failures)
		}
		if msg.resetMainQueue {
			m.queue = job.CreateJobQueue()
		}
		if m.updateJobsRunningFlag() {
			return m, tickCmd()
		}
		return m, nil

	case repoActionResultMsg:
		m.ensureSelectionWithinBounds(msg.panel)
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case autoFetchFailedMsg:
		m.jobsRunning = false
		if len(msg.names) > 0 {
			m.err = fmt.Errorf("auto fetch failed for: %s", strings.Join(msg.names, ", "))
		}
		return m, nil
	}

	return m, nil
} // handleKeyPress processes keyboard input
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.activeCredentialPrompt != nil {
		handled, cmd := m.handleCredentialPromptKey(msg)
		if handled {
			return m, cmd
		}
	}

	if m.activeForcePrompt != nil {
		switch key {
		case "y", "Y", "enter":
			cmd := m.confirmForcePush()
			return m, cmd
		case "n", "N", "esc":
			m.dismissForcePrompt()
			return m, nil
		default:
			return m, nil
		}
	}

	// Global keybindings
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
		if m.currentView == FocusView {
			m.currentView = OverviewView
			m.sidePanel = NonePanel
			return m, nil
		}
		return m, nil

	case "tab":
		// TAB opens lazygit for the currently selected repository
		if len(m.repositories) > 0 && m.cursor < len(m.repositories) {
			r := m.repositories[m.cursor]
			if isLazygitAvailable() {
				return m, tea.ExecProcess(exec.Command("lazygit", "-p", r.AbsPath), func(err error) tea.Msg {
					if err != nil {
						return errMsg{err: err}
					}
					// Refresh repository state after lazygit exits
					if refreshErr := r.ForceRefresh(); refreshErr != nil {
						// Continue even if refresh fails
					}
					return lazygitClosedMsg{}
				})
			}
		}
		return m, nil
	}

	// View-specific keybindings
	switch m.currentView {
	case OverviewView:
		return m.handleOverviewKeys(msg)
	case FocusView:
		return m.handleFocusKeys(msg)
	}

	return m, nil
}

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
		m.toggleCredentialField()
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
	if m.credentialInputBuffer == "" {
		return
	}
	runes := []rune(m.credentialInputBuffer)
	if len(runes) == 0 {
		return
	}
	m.credentialInputBuffer = string(runes[:len(runes)-1])
}

func (m *Model) toggleCredentialField() {
	if m.activeCredentialPrompt == nil {
		return
	}
	if m.credentialInputField == credentialFieldUsername {
		m.activeCredentialPrompt.username = strings.TrimSpace(m.credentialInputBuffer)
		m.credentialInputField = credentialFieldPassword
		m.credentialInputBuffer = m.activeCredentialPrompt.password
		return
	}
	m.activeCredentialPrompt.password = m.credentialInputBuffer
	m.credentialInputField = credentialFieldUsername
	m.credentialInputBuffer = m.activeCredentialPrompt.username
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

func (m *Model) enqueueCredentialPrompt(j *job.Job) {
	if j == nil || j.Repository == nil {
		return
	}

	if m.activeCredentialPrompt != nil && m.activeCredentialPrompt.repo != nil && m.activeCredentialPrompt.repo.RepoID == j.Repository.RepoID {
		m.activeCredentialPrompt.job = j
		if creds := credentialsFromJob(j); creds != nil {
			m.activeCredentialPrompt.username = creds.User
			if m.credentialInputField == credentialFieldUsername {
				m.credentialInputBuffer = creds.User
			}
		}
		return
	}

	for _, pending := range m.credentialPromptQueue {
		if pending == nil || pending.repo == nil {
			continue
		}
		if pending.repo.RepoID == j.Repository.RepoID {
			pending.job = j
			if creds := credentialsFromJob(j); creds != nil {
				pending.username = creds.User
			}
			return
		}
	}

	prompt := &credentialPrompt{
		repo: j.Repository,
		job:  j,
	}
	if creds := credentialsFromJob(j); creds != nil {
		prompt.username = creds.User
	}
	m.credentialPromptQueue = append(m.credentialPromptQueue, prompt)
	if m.activeCredentialPrompt == nil {
		m.advanceCredentialPrompt()
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
	queue := job.CreateJobQueue()
	if err := queue.AddJob(retryJob); err != nil {
		repo.SetWorkStatus(git.Fail)
		if repo.State != nil {
			repo.State.Message = "failed to queue credential retry"
		}
		return func() tea.Msg { return errMsg{err: err} }
	}
	repo.SetWorkStatus(git.Queued)
	m.jobsRunning = true
	return tea.Batch(runJobQueueCmd(queue, false), tickCmd())
}

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
			opts = &copyCfg
		case command.FetchOptions:
			copyCfg := cfg
			copyCfg.Credentials = creds
			opts = &copyCfg
		default:
			opts = &command.FetchOptions{
				RemoteName:  defaultRemoteName(original.Repository),
				CommandMode: command.ModeNative,
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

func credentialsFromJob(j *job.Job) *git.Credentials {
	if j == nil {
		return nil
	}
	switch cfg := j.Options.(type) {
	case *command.FetchOptions:
		return cfg.Credentials
	case command.FetchOptions:
		return cfg.Credentials
	case *command.PullOptions:
		return cfg.Credentials
	case command.PullOptions:
		return cfg.Credentials
	case *job.PullJobConfig:
		if cfg.Options != nil {
			return cfg.Options.Credentials
		}
	case job.PullJobConfig:
		if cfg.Options != nil {
			return cfg.Options.Credentials
		}
	}
	return nil
}

// handleOverviewKeys processes keys in overview mode
func (m *Model) handleOverviewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if len(m.repositories) == 0 {
			return m, nil
		}
		m.cursor = (m.cursor - 1 + len(m.repositories)) % len(m.repositories)
		m.cursor = m.findNextReadyIndex(m.cursor, -1)
		return m, nil

	case "down", "j":
		if len(m.repositories) == 0 {
			return m, nil
		}
		m.cursor = (m.cursor + 1) % len(m.repositories)
		m.cursor = m.findNextReadyIndex(m.cursor, 1)
		return m, nil

	case "g": // First g of gg - we need to check if it's followed by another g
		// For now, just go to top (single g also works)
		m.cursor = 0

	case "G": // Shift+G goes to end
		if len(m.repositories) > 0 {
			m.cursor = len(m.repositories) - 1
		}

	case "home":
		m.cursor = 0

	case "end":
		if len(m.repositories) > 0 {
			m.cursor = len(m.repositories) - 1
		}

	case "ctrl+f", "pgdown": // Ctrl+F and Page Down - scroll forward (down)
		pageSize := m.height - 5
		m.cursor += pageSize
		if m.cursor >= len(m.repositories) {
			m.cursor = len(m.repositories) - 1
		}
		m.cursor = m.findNextReadyIndex(m.cursor, 1)

	case "ctrl+b", "pgup": // Ctrl+B and Page Up - scroll backward (up)
		pageSize := m.height - 5
		m.cursor -= pageSize
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.cursor = m.findNextReadyIndex(m.cursor, -1)

	case "ctrl+d": // Ctrl+D - scroll down half page
		halfPage := (m.height - 5) / 2
		m.cursor += halfPage
		if m.cursor >= len(m.repositories) {
			m.cursor = len(m.repositories) - 1
		}
		m.cursor = m.findNextReadyIndex(m.cursor, 1)

	case "ctrl+u": // Ctrl+U - scroll up half page
		halfPage := (m.height - 5) / 2
		m.cursor -= halfPage
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.cursor = m.findNextReadyIndex(m.cursor, -1)

	case " ", "space":
		return m, m.toggleQueue()

	case "a":
		return m, m.queueAll()

	case "A":
		return m, m.unqueueAll()

	case "enter":
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

	case "c":
		if len(m.repositories) > 0 && m.cursor < len(m.repositories) {
			repo := m.repositories[m.cursor]
			if repo != nil && repo.State != nil && repo.WorkStatus() == git.Fail && repo.State.Message != "" {
				repo.State.Message = ""
				repo.SetWorkStatus(git.Available)
				return m, nil
			}
		}
		if m.err != nil {
			m.err = nil
			return m, nil
		}
		if m.hasMultipleTagged() {
			m.notifyMultiSelectionRestriction("Commit view unavailable for tagged selection")
			return m, nil
		}
		m.activatePanel(CommitPanel)

	case "r":
		m.activatePanel(RemotePanel)

	case "s":
		if m.hasMultipleTagged() {
			m.notifyMultiSelectionRestriction("Status view unavailable for tagged selection")
			return m, nil
		}
		m.activatePanel(StatusPanel)

	case "S":
		if m.hasMultipleTagged() {
			m.notifyMultiSelectionRestriction("Stash view unavailable for tagged selection")
			return m, nil
		}
		m.activatePanel(StashPanel)

	case "n":
		m.sortByName()

	case "t":
		m.sortByTime()
	}

	return m, nil
}

// handleFocusKeys processes keys in focus mode
func (m *Model) handleFocusKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc", "backspace":
		m.currentView = OverviewView
		m.sidePanel = NonePanel
		return m, nil
	}

	switch m.sidePanel {
	case BranchPanel:
		return m.handleBranchPanelKey(key)
	case RemotePanel:
		return m.handleRemotePanelKey(key)
	case CommitPanel:
		return m.handleCommitPanelKey(key)
	default:
		return m, nil
	}
}

// addRepository adds a repository to the model in sorted order
func (m *Model) addRepository(r *git.Repository) {
	rs := m.repositories
	index := sort.Search(len(rs), func(i int) bool { return git.Less(r, rs[i]) })
	rs = append(rs, &git.Repository{})
	copy(rs[index+1:], rs[index:])
	rs[index] = r

	// Add listeners
	r.On(git.RepositoryUpdated, func(event *git.RepositoryEvent) error {
		// Repository updated - could trigger a re-render if needed
		return nil
	})
	r.On(git.BranchUpdated, func(event *git.RepositoryEvent) error {
		// Branch updated - could trigger a re-render if needed
		return nil
	})

	m.repositories = rs
}

func (m *Model) currentRepository() *git.Repository {
	if len(m.repositories) == 0 {
		return nil
	}
	if m.cursor < 0 || m.cursor >= len(m.repositories) {
		return nil
	}
	return m.repositories[m.cursor]
}

func repoHasActiveJob(status git.WorkStatus) bool {
	return status == git.Pending || status == git.Queued || status == git.Working
}

func repoIsDirty(repo *git.Repository) bool {
	if repo == nil {
		return false
	}
	if repo.State == nil || repo.State.Branch == nil {
		return false
	}
	return !repo.State.Branch.Clean
}

func repoIsActionable(repo *git.Repository) bool {
	if repo == nil {
		return false
	}
	if !repo.WorkStatus().Ready {
		return false
	}
	if repo.State == nil || repo.State.Branch == nil {
		return false
	}
	return repo.State.Branch.Clean
}

func (m *Model) processJobFailures(fails map[*job.Job]error) {
	for j, err := range fails {
		if err == nil {
			continue
		}
		if err == gerr.ErrAuthenticationRequired || err == gerr.ErrAuthorizationFailed {
			j.Repository.SetWorkStatus(git.Paused)
			if j.Repository.State != nil {
				j.Repository.State.Message = "credentials required"
			}
			m.enqueueCredentialPrompt(j)
			continue
		}
		if j.JobType == job.PushJob {
			j.Repository.SetWorkStatus(git.Fail)
			m.enqueueForcePrompt(j.Repository)
		}
	}
}

// toggleQueue adds/removes repository from queue
func (m *Model) toggleQueue() tea.Cmd {
	if len(m.repositories) == 0 {
		return nil
	}

	r := m.repositories[m.cursor]
	if r == nil {
		return nil
	}
	switch r.WorkStatus() {
	case git.Queued:
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

// addToQueue adds a repository to the job queue
func (m *Model) addToQueue(r *git.Repository) error {
	if !repoIsActionable(r) {
		return nil
	}

	j := &job.Job{
		Repository: r,
	}

	switch m.mode.ID {
	case PullMode:
		if r.State.Branch.Upstream == nil {
			return nil
		}
		if r.State.Remote == nil {
			return nil
		}
		j.JobType = job.PullJob
		j.Options = &command.PullOptions{
			RemoteName:  r.State.Remote.Name,
			CommandMode: command.ModeLegacy,
			FFOnly:      true,
		}
	case MergeMode:
		if r.State.Branch.Upstream == nil {
			return nil
		}
		j.JobType = job.MergeJob
	case RebaseMode:
		if r.State.Branch.Upstream == nil || r.State.Remote == nil {
			return nil
		}
		j.JobType = job.RebaseJob
		j.Options = &command.PullOptions{
			RemoteName:  r.State.Remote.Name,
			CommandMode: command.ModeLegacy,
			Rebase:      true,
		}
	case PushMode:
		if r.State.Remote == nil || r.State.Branch == nil {
			return nil
		}
		j.JobType = job.PushJob
		j.Options = &command.PushOptions{
			RemoteName:    r.State.Remote.Name,
			ReferenceName: r.State.Branch.Name,
			CommandMode:   command.ModeLegacy,
		}
	default:
		return nil
	}

	if err := m.queue.AddJob(j); err != nil {
		return err
	}
	r.SetWorkStatus(git.Queued)

	return nil
}

// removeFromQueue removes a repository from the job queue
func (m *Model) removeFromQueue(r *git.Repository) error {
	if err := m.queue.RemoveFromQueue(r); err != nil {
		return err
	}
	r.SetWorkStatus(git.Available)
	return nil
}

func (m *Model) runFetchForRepo(repo *git.Repository) tea.Cmd {
	if repo == nil {
		return nil
	}
	if !repoIsActionable(repo) {
		return nil
	}
	if repoHasActiveJob(repo.WorkStatus()) {
		return nil
	}
	return m.startFetchForRepos([]*git.Repository{repo})
}

func (m *Model) runPullForRepo(repo *git.Repository, suppressSuccess bool) tea.Cmd {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return nil
	}
	if !repoIsActionable(repo) {
		return nil
	}
	if repoHasActiveJob(repo.WorkStatus()) {
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
	pullCfg := &job.PullJobConfig{
		Options: &command.PullOptions{
			RemoteName:  repo.State.Remote.Name,
			CommandMode: command.ModeLegacy,
			FFOnly:      true,
		},
		SuppressSuccess: suppressSuccess,
	}
	jobQueue := job.CreateJobQueue()
	jobEntry := &job.Job{
		Repository: repo,
		JobType:    job.PullJob,
		Options:    pullCfg,
	}
	if err := jobQueue.AddJob(jobEntry); err != nil {
		repo.SetWorkStatus(git.Available)
		return func() tea.Msg { return errMsg{err: err} }
	}
	repo.SetWorkStatus(git.Queued)
	m.jobsRunning = true
	return tea.Batch(runJobQueueCmd(jobQueue, false), tickCmd())
}

func (m *Model) runPushForRepo(repo *git.Repository, force bool, suppressSuccess bool, message string) tea.Cmd {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return nil
	}
	if repoHasActiveJob(repo.WorkStatus()) {
		return nil
	}
	if !repoIsActionable(repo) {
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
	pushCfg := &job.PushJobConfig{
		Options: &command.PushOptions{
			RemoteName:    repo.State.Remote.Name,
			ReferenceName: repo.State.Branch.Name,
			CommandMode:   command.ModeLegacy,
			Force:         force,
		},
		SuppressSuccess: suppressSuccess,
	}
	jobQueue := job.CreateJobQueue()
	jobEntry := &job.Job{
		Repository: repo,
		JobType:    job.PushJob,
		Options:    pushCfg,
	}
	if err := jobQueue.AddJob(jobEntry); err != nil {
		repo.SetWorkStatus(git.Available)
		return func() tea.Msg { return errMsg{err: err} }
	}
	repo.SetWorkStatus(git.Queued)
	m.jobsRunning = true
	return tea.Batch(runJobQueueCmd(jobQueue, false), tickCmd())
}

func (m *Model) enqueueForcePrompt(repo *git.Repository) {
	if repo == nil {
		return
	}
	if m.activeForcePrompt != nil && m.activeForcePrompt.repo.RepoID == repo.RepoID {
		return
	}
	for _, pending := range m.forcePromptQueue {
		if pending == nil {
			continue
		}
		if pending.repo != nil && pending.repo.RepoID == repo.RepoID {
			return
		}
	}
	m.forcePromptQueue = append(m.forcePromptQueue, &forcePushPrompt{repo: repo})
	if m.activeForcePrompt == nil {
		m.advanceForcePrompt()
	}
}

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
	// Move to next prompt regardless of outcome
	m.dismissForcePrompt()
	if repo == nil {
		return nil
	}
	return m.runPushForRepo(repo, true, true, "retrying push with --force")
}

// queueAll adds all available repositories to the queue
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

// unqueueAll removes all repositories from the queue
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

// startQueue starts processing the job queue
func (m *Model) startQueue() tea.Cmd {
	m.jobsRunning = true
	currentQueue := m.queue
	return tea.Batch(runJobQueueCmd(currentQueue, true), tickCmd())
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
		if repo.State.Remote == nil {
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
		queue := job.CreateJobQueue()
		failures := make([]string, 0)
		for _, repo := range repos {
			opts := &command.FetchOptions{
				RemoteName:  defaultRemoteName(repo),
				CommandMode: command.ModeNative,
			}
			if err := queue.AddJob(&job.Job{
				JobType:    job.FetchJob,
				Repository: repo,
				Options:    opts,
			}); err != nil {
				failures = append(failures, repo.Name)
				continue
			}
		}

		fails := queue.StartJobsAsync()
		for j := range fails {
			if j == nil || j.Repository == nil {
				continue
			}
			failures = append(failures, j.Repository.Name)
		}
		if len(failures) > 0 {
			sort.Strings(failures)
			return autoFetchFailedMsg{names: failures}
		}

		return jobCompletedMsg{}
	}
}

func defaultRemoteName(repo *git.Repository) string {
	if repo != nil && repo.State.Remote != nil && repo.State.Remote.Name != "" {
		return repo.State.Remote.Name
	}
	return "origin"
}

func (m *Model) advanceSpinner() {
	if len(spinnerFrames) == 0 {
		return
	}
	m.spinnerIndex = (m.spinnerIndex + 1) % len(spinnerFrames)
}

func (m *Model) updateJobsRunningFlag() bool {
	for _, r := range m.repositories {
		status := r.WorkStatus()
		if status == git.Working || status == git.Pending {
			m.jobsRunning = true
			return true
		}
	}
	m.jobsRunning = false
	return false
}

func (m *Model) findNextReadyIndex(start int, direction int) int {
	count := len(m.repositories)
	if count == 0 {
		return 0
	}
	index := start
	for i := 0; i < count; i++ {
		// Normalize index to remain inside bounds
		if index < 0 {
			index = count - 1
		}
		if index >= count {
			index = 0
		}
		repo := m.repositories[index]
		if repo != nil && (repo.WorkStatus().Ready || repoIsDirty(repo)) {
			return index
		}
		index += direction
	}
	// No ready repositories, keep original index but clamp to within range
	if start < 0 {
		return 0
	}
	if start >= count {
		return count - 1
	}
	return start
}

func runJobQueueCmd(queue *job.Queue, resetMainQueue bool) tea.Cmd {
	return func() tea.Msg {
		failures := queue.StartJobsAsync()
		return jobQueueResultMsg{
			resetMainQueue: resetMainQueue,
			failures:       failures,
		}
	}
}

// cycleMode cycles through available modes
func (m *Model) cycleMode() {
	for i, mode := range modes {
		if mode.ID == m.mode.ID {
			m.mode = modes[(i+1)%len(modes)]
			return
		}
	}
}

// sortByName sorts repositories alphabetically
func (m *Model) sortByName() {
	sort.Sort(git.Alphabetical(m.repositories))
}

// sortByTime sorts repositories by last modified time
func (m *Model) sortByTime() {
	sort.Sort(git.LastModified(m.repositories))
}

func (m *Model) handleBranchPanelKey(key string) (tea.Model, tea.Cmd) {
	items := m.branchPanelItems()
	count := len(items)
	if count == 0 {
		return m, nil
	}

	switch key {
	case "up", "k":
		wrapCursor(&m.branchCursor, count, -1)
	case "down", "j":
		wrapCursor(&m.branchCursor, count, 1)
	case "home", "g":
		m.branchCursor = 0
	case "end", "G":
		m.branchCursor = count - 1
	case "enter", "c":
		branchName := items[clampIndex(m.branchCursor, count)].Name
		if branchName == "" || branchName == "<unknown>" {
			return m, nil
		}
		if m.hasMultipleTagged() {
			return m, m.checkoutBranchMultiCmd(m.taggedRepositories(), branchName)
		}
		repos := m.panelRepositories()
		if len(repos) == 0 {
			return m, nil
		}
		branch := findBranchByName(repos[0], branchName)
		return m, m.checkoutBranchCmd(repos[0], branch)
	case "d":
		branchName := items[clampIndex(m.branchCursor, count)].Name
		if branchName == "" || branchName == "<unknown>" {
			return m, nil
		}
		if m.hasMultipleTagged() {
			return m, m.deleteBranchMultiCmd(m.taggedRepositories(), branchName)
		}
		repos := m.panelRepositories()
		if len(repos) == 0 {
			return m, nil
		}
		repo := repos[0]
		if repo.State != nil && repo.State.Branch != nil && repo.State.Branch.Name == branchName {
			repo.State.Message = "cannot delete current branch"
			return m, nil
		}
		branch := findBranchByName(repo, branchName)
		return m, m.deleteBranchCmd(repo, branch)
	}

	return m, nil
}

func (m *Model) handleRemotePanelKey(key string) (tea.Model, tea.Cmd) {
	items := m.remotePanelItems()
	count := len(items)
	if count == 0 {
		return m, nil
	}

	switch key {
	case "up", "k":
		wrapCursor(&m.remoteBranchCursor, count, -1)
	case "down", "j":
		wrapCursor(&m.remoteBranchCursor, count, 1)
	case "home", "g":
		m.remoteBranchCursor = 0
	case "end", "G":
		m.remoteBranchCursor = count - 1
	case "enter", "c":
		entry := items[clampIndex(m.remoteBranchCursor, count)]
		if m.hasMultipleTagged() {
			return m, m.checkoutRemoteBranchMultiCmd(m.taggedRepositories(), entry)
		}
		repos := m.panelRepositories()
		if len(repos) == 0 {
			return m, nil
		}
		return m, m.checkoutRemoteBranchCmd(repos[0], entry)
	case "d":
		entry := items[clampIndex(m.remoteBranchCursor, count)]
		if m.hasMultipleTagged() {
			return m, m.deleteRemoteBranchMultiCmd(m.taggedRepositories(), entry)
		}
		repos := m.panelRepositories()
		if len(repos) == 0 {
			return m, nil
		}
		return m, m.deleteRemoteBranchCmd(repos[0], entry)
	}

	return m, nil
}

func (m *Model) handleCommitPanelKey(key string) (tea.Model, tea.Cmd) {
	if m.hasMultipleTagged() {
		return m, nil
	}

	repo := m.currentRepository()
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return m, nil
	}

	if len(repo.State.Branch.Commits) == 0 {
		if err := repo.State.Branch.InitializeCommits(repo); err != nil {
			repo.State.Message = err.Error()
			return m, nil
		}
	}

	commits := repo.State.Branch.Commits
	count := len(commits)
	if count == 0 {
		return m, nil
	}
	viewport := m.commitViewportSize()
	if viewport > count {
		viewport = count
	}

	switch key {
	case "up", "k":
		wrapCursor(&m.commitCursor, count, -1)
	case "down", "j":
		wrapCursor(&m.commitCursor, count, 1)
	case "home", "g":
		m.commitCursor = 0
	case "end", "G":
		m.commitCursor = count - 1
	case "ctrl+f", "pgdown":
		m.commitCursor += viewport
		if m.commitCursor >= count {
			m.commitCursor = count - 1
		}
	case "ctrl+b", "pgup":
		m.commitCursor -= viewport
		if m.commitCursor < 0 {
			m.commitCursor = 0
		}
	case "ctrl+d":
		half := viewport / 2
		if half < 1 {
			half = 1
		}
		m.commitCursor += half
		if m.commitCursor >= count {
			m.commitCursor = count - 1
		}
	case "ctrl+u":
		half := viewport / 2
		if half < 1 {
			half = 1
		}
		m.commitCursor -= half
		if m.commitCursor < 0 {
			m.commitCursor = 0
		}
	case "enter", "c":
		commit := commits[clampIndex(m.commitCursor, count)]
		return m, m.checkoutCommitCmd(repo, commit)
	case "s":
		commit := commits[clampIndex(m.commitCursor, count)]
		return m, m.resetToCommitCmd(repo, commit, command.ResetSoft)
	case "m":
		commit := commits[clampIndex(m.commitCursor, count)]
		return m, m.resetToCommitCmd(repo, commit, command.ResetMixed)
	case "h":
		commit := commits[clampIndex(m.commitCursor, count)]
		return m, m.resetToCommitCmd(repo, commit, command.ResetHard)
	}

	m.ensureCommitCursorVisible(count, viewport)

	return m, nil
}

func (m *Model) activatePanel(panel SidePanelType) {
	m.sidePanel = panel
	if panel == NonePanel {
		m.currentView = OverviewView
		return
	}

	m.currentView = FocusView
	repo := m.currentRepository()

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
	case CommitPanel:
		if repo != nil && repo.State != nil && repo.State.Branch != nil {
			if len(repo.State.Branch.Commits) == 0 {
				_ = repo.State.Branch.InitializeCommits(repo)
			}
			if len(repo.State.Branch.Commits) > 0 {
				m.commitOffset = 0
				m.commitCursor = clampIndex(m.commitCursor, len(repo.State.Branch.Commits))
				viewport := m.commitViewportSize()
				if viewport > len(repo.State.Branch.Commits) {
					viewport = len(repo.State.Branch.Commits)
				}
				m.ensureCommitCursorVisible(len(repo.State.Branch.Commits), viewport)
			}
		}
	}

	m.ensureSelectionWithinBounds(panel)
}

func (m *Model) ensureSelectionWithinBounds(panel SidePanelType) {
	switch panel {
	case BranchPanel:
		length := len(m.branchPanelItems())
		m.branchCursor = clampIndex(m.branchCursor, length)
	case RemotePanel:
		length := len(m.remotePanelItems())
		m.remoteBranchCursor = clampIndex(m.remoteBranchCursor, length)
	case CommitPanel:
		repo := m.currentRepository()
		length := 0
		if repo != nil && repo.State != nil && repo.State.Branch != nil {
			length = len(repo.State.Branch.Commits)
		}
		m.commitCursor = clampIndex(m.commitCursor, length)
		viewport := m.commitViewportSize()
		if viewport > length {
			viewport = length
		}
		m.ensureCommitCursorVisible(length, viewport)
	}
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
		if repo == nil || repo.State == nil {
			continue
		}
		repo.State.Message = message
	}
}

func clampIndex(idx int, length int) int {
	if length <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= length {
		return length - 1
	}
	return idx
}

func wrapCursor(cursor *int, length int, delta int) {
	if length <= 0 {
		*cursor = 0
		return
	}
	*cursor += delta
	for *cursor < 0 {
		*cursor += length
	}
	for *cursor >= length {
		*cursor -= length
	}
}

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
		if err := refreshBranchState(repo); err != nil {
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
		args := []string{"branch", "-d", branch.Name}
		if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("delete branch %s: %w", branch.Name, err)}
		}
		repo.State.Message = fmt.Sprintf("deleted %s", branch.Name)
		if err := refreshBranchState(repo); err != nil {
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
			branch := branchLookup[repo]
			repo.State.Message = fmt.Sprintf("checking out %s", branchName)
			if err := repo.Checkout(branch); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("checkout branch %s in %s: %w", branchName, repo.Name, err)}
			}
			repo.State.Message = fmt.Sprintf("switched to %s", branchName)
			if err := refreshBranchState(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: BranchPanel}
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
			args := []string{"branch", "-d", branchName}
			if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("delete branch %s in %s: %w", branchName, repo.Name, err)}
			}
			repo.State.Message = fmt.Sprintf("deleted %s", branchName)
			if err := refreshBranchState(repo); err != nil {
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
		if err := refreshBranchState(repo); err != nil {
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
		if err := refreshBranchState(repo); err != nil {
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
			if err := refreshBranchState(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: RemotePanel}
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
			if err := refreshBranchState(repo); err != nil {
				return errMsg{err: fmt.Errorf("refresh repository %s: %w", repo.Name, err)}
			}
		}
		return repoActionResultMsg{panel: RemotePanel}
	}
}

func (m *Model) checkoutCommitCmd(repo *git.Repository, commit *git.Commit) tea.Cmd {
	if repo == nil || commit == nil {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("checking out %s", shortHash(commit.Hash))
		args := []string{"checkout", commit.Hash}
		if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("checkout commit %s: %w", commit.Hash, err)}
		}
		repo.State.Message = fmt.Sprintf("checked out %s", shortHash(commit.Hash))
		if err := refreshBranchState(repo); err != nil {
			return errMsg{err: err}
		}
		return repoActionResultMsg{panel: CommitPanel}
	}
}

func (m *Model) resetToCommitCmd(repo *git.Repository, commit *git.Commit, resetType command.ResetType) tea.Cmd {
	if repo == nil || commit == nil {
		return nil
	}
	return func() tea.Msg {
		repo.State.Message = fmt.Sprintf("reset --%s %s", resetType, shortHash(commit.Hash))
		args := []string{"reset", "--" + string(resetType), commit.Hash}
		if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("reset --%s %s: %w", resetType, commit.Hash, err)}
		}
		repo.State.Message = fmt.Sprintf("reset --%s %s", resetType, shortHash(commit.Hash))
		if err := refreshBranchState(repo); err != nil {
			return errMsg{err: err}
		}
		return repoActionResultMsg{panel: CommitPanel}
	}
}

func (m *Model) commitViewportSize() int {
	height := m.height
	if height <= 0 {
		height = 24
	}
	viewport := height - 8
	if viewport < 1 {
		viewport = 1
	}
	return viewport
}

func (m *Model) ensureCommitCursorVisible(total, viewport int) {
	if total <= 0 {
		m.commitCursor = 0
		m.commitOffset = 0
		return
	}
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
	if m.commitCursor >= total {
		m.commitCursor = total - 1
	}
	if viewport <= 0 {
		viewport = 1
	}
	if viewport > total {
		viewport = total
	}
	maxOffset := total - viewport
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.commitOffset < 0 {
		m.commitOffset = 0
	}
	if m.commitOffset > maxOffset {
		m.commitOffset = maxOffset
	}
	if m.commitCursor < m.commitOffset {
		m.commitOffset = m.commitCursor
	} else if m.commitCursor >= m.commitOffset+viewport {
		m.commitOffset = m.commitCursor - viewport + 1
	}
	if m.commitOffset < 0 {
		m.commitOffset = 0
	}
	if m.commitOffset > maxOffset {
		m.commitOffset = maxOffset
	}
}
