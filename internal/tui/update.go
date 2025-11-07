package tui

import (
	"fmt"
	"os/exec"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
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
		for _, repo := range msg.repos {
			m.addRepository(repo)
		}
		m.loading = false
		return m, nil

	case lazygitClosedMsg:
		// Lazygit has closed, just refresh the display
		return m, nil

	case jobCompletedMsg:
		// Check if any jobs are still running
		stillRunning := false
		for _, r := range m.repositories {
			if r.WorkStatus() == git.Working {
				stillRunning = true
				break
			}
		}

		if stillRunning {
			// Jobs still running, send another tick
			return m, tickCmd()
		} else {
			// All jobs completed
			m.jobsRunning = false
			return m, nil
		}

	case repoActionResultMsg:
		m.ensureSelectionWithinBounds(msg.panel)
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}

	return m, nil
} // handleKeyPress processes keyboard input
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keybindings
	switch msg.String() {
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
	if m.currentView == OverviewView {
		return m.handleOverviewKeys(msg)
	} else if m.currentView == FocusView {
		return m.handleFocusKeys(msg)
	}

	return m, nil
}

// handleOverviewKeys processes keys in overview mode
func (m *Model) handleOverviewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		} else {
			m.cursor = len(m.repositories) - 1
		}

	case "down", "j":
		if m.cursor < len(m.repositories)-1 {
			m.cursor++
		} else {
			m.cursor = 0
		}

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

	case "ctrl+b", "pgup": // Ctrl+B and Page Up - scroll backward (up)
		pageSize := m.height - 5
		m.cursor -= pageSize
		if m.cursor < 0 {
			m.cursor = 0
		}

	case "ctrl+d": // Ctrl+D - scroll down half page
		halfPage := (m.height - 5) / 2
		m.cursor += halfPage
		if m.cursor >= len(m.repositories) {
			m.cursor = len(m.repositories) - 1
		}

	case "ctrl+u": // Ctrl+U - scroll up half page
		halfPage := (m.height - 5) / 2
		m.cursor -= halfPage
		if m.cursor < 0 {
			m.cursor = 0
		}

	case " ", "space":
		return m, m.toggleQueue()

	case "a":
		return m, m.queueAll()

	case "A":
		return m, m.unqueueAll()

	case "enter":
		return m, m.startQueue()

	case "m":
		m.cycleMode()

	case "b":
		m.activatePanel(BranchPanel)

	case "c":
		m.activatePanel(CommitPanel)

	case "r":
		m.activatePanel(RemotePanel)

	case "s":
		m.activatePanel(StatusPanel)

	case "S":
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

// toggleQueue adds/removes repository from queue
func (m *Model) toggleQueue() tea.Cmd {
	if len(m.repositories) == 0 {
		return nil
	}

	r := m.repositories[m.cursor]

	if r.WorkStatus().Ready {
		return func() tea.Msg {
			m.addToQueue(r)
			return jobCompletedMsg{}
		}
	} else if r.WorkStatus() == git.Queued {
		return func() tea.Msg {
			m.removeFromQueue(r)
			return jobCompletedMsg{}
		}
	}

	return nil
}

// addToQueue adds a repository to the job queue
func (m *Model) addToQueue(r *git.Repository) error {
	j := &job.Job{
		Repository: r,
	}

	switch m.mode.ID {
	case FetchMode:
		j.JobType = job.FetchJob
	case PullMode:
		if r.State.Branch.Upstream == nil {
			return nil
		}
		j.JobType = job.PullJob
	case MergeMode:
		if r.State.Branch.Upstream == nil {
			return nil
		}
		j.JobType = job.MergeJob
	case CheckoutMode:
		j.JobType = job.CheckoutJob
		j.Options = &command.CheckoutOptions{
			TargetRef:      m.targetBranch,
			CreateIfAbsent: true,
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

// queueAll adds all available repositories to the queue
func (m *Model) queueAll() tea.Cmd {
	return func() tea.Msg {
		for _, r := range m.repositories {
			if r.WorkStatus().Ready {
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

	// Start jobs in a goroutine
	go func() {
		fails := m.queue.StartJobsAsync()
		m.queue = job.CreateJobQueue()
		for j, err := range fails {
			// Handle authentication failures
			if err != nil {
				j.Repository.SetWorkStatus(git.Paused)
				_ = m.failoverQueue.AddJob(j)
			}
		}
	}()

	// Start the tick command to refresh the UI periodically
	return tickCmd()
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
	repo := m.currentRepository()
	if repo == nil {
		return m, nil
	}
	count := len(repo.Branches)
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
		branch := repo.Branches[clampIndex(m.branchCursor, count)]
		return m, m.checkoutBranchCmd(repo, branch)
	case "d":
		branch := repo.Branches[clampIndex(m.branchCursor, count)]
		if repo.State != nil && repo.State.Branch != nil && branch != nil && branch.Name == repo.State.Branch.Name {
			repo.State.Message = "cannot delete current branch"
			return m, nil
		}
		return m, m.deleteBranchCmd(repo, branch)
	}

	return m, nil
}

func (m *Model) handleRemotePanelKey(key string) (tea.Model, tea.Cmd) {
	repo := m.currentRepository()
	if repo == nil {
		return m, nil
	}
	items := remoteBranchItems(repo)
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
		item := items[clampIndex(m.remoteBranchCursor, count)]
		return m, m.checkoutRemoteBranchCmd(repo, item)
	case "d":
		item := items[clampIndex(m.remoteBranchCursor, count)]
		return m, m.deleteRemoteBranchCmd(repo, item)
	}

	return m, nil
}

func (m *Model) handleCommitPanelKey(key string) (tea.Model, tea.Cmd) {
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
		if repo != nil && repo.State != nil && repo.State.Branch != nil {
			if idx := branchIndex(repo.Branches, repo.State.Branch.Name); idx >= 0 {
				m.branchCursor = idx
			}
		}
	case RemotePanel:
		if repo != nil && repo.State != nil && repo.State.Branch != nil && repo.State.Branch.Upstream != nil {
			items := remoteBranchItems(repo)
			if idx := remoteBranchIndex(items, repo.State.Branch.Upstream.Name); idx >= 0 {
				m.remoteBranchCursor = idx
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
	repo := m.currentRepository()
	switch panel {
	case BranchPanel:
		length := 0
		if repo != nil {
			length = len(repo.Branches)
		}
		m.branchCursor = clampIndex(m.branchCursor, length)
	case RemotePanel:
		length := 0
		if repo != nil {
			length = len(remoteBranchItems(repo))
		}
		m.remoteBranchCursor = clampIndex(m.remoteBranchCursor, length)
	case CommitPanel:
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

func (m *Model) currentRepository() *git.Repository {
	if len(m.repositories) == 0 {
		return nil
	}
	if m.cursor < 0 || m.cursor >= len(m.repositories) {
		return nil
	}
	return m.repositories[m.cursor]
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
		if err := repo.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		if repo.State != nil && repo.State.Branch != nil {
			_ = repo.State.Branch.InitializeCommits(repo)
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
		if err := repo.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		if repo.State != nil && repo.State.Branch != nil {
			_ = repo.State.Branch.InitializeCommits(repo)
		}
		return repoActionResultMsg{panel: BranchPanel}
	}
}

func (m *Model) checkoutRemoteBranchCmd(repo *git.Repository, item remoteBranchItem) tea.Cmd {
	if repo == nil || item.branch == nil {
		return nil
	}
	return func() tea.Msg {
		branchName := remoteBranchShortName(item)
		repo.State.Message = fmt.Sprintf("checking out %s", branchName)
		if existing := findBranchByName(repo, branchName); existing != nil {
			if err := repo.Checkout(existing); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("checkout branch %s: %w", existing.Name, err)}
			}
		} else {
			args := []string{"checkout", "-b", branchName, item.branch.Name}
			if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
				repo.State.Message = err.Error()
				return errMsg{err: fmt.Errorf("create branch %s from %s: %w", branchName, item.branch.Name, err)}
			}
		}
		repo.State.Message = fmt.Sprintf("switched to %s", branchName)
		if err := repo.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		if repo.State != nil && repo.State.Branch != nil {
			_ = repo.State.Branch.InitializeCommits(repo)
		}
		return repoActionResultMsg{panel: RemotePanel}
	}
}

func (m *Model) deleteRemoteBranchCmd(repo *git.Repository, item remoteBranchItem) tea.Cmd {
	if repo == nil || item.branch == nil || item.remote == nil {
		return nil
	}
	return func() tea.Msg {
		shortName := remoteBranchShortName(item)
		repo.State.Message = fmt.Sprintf("deleting %s/%s", item.remote.Name, shortName)
		args := []string{"push", item.remote.Name, "--delete", shortName}
		if _, err := command.Run(repo.AbsPath, "git", args); err != nil {
			repo.State.Message = err.Error()
			return errMsg{err: fmt.Errorf("delete remote branch %s/%s: %w", item.remote.Name, shortName, err)}
		}
		repo.State.Message = fmt.Sprintf("deleted %s/%s", item.remote.Name, shortName)
		if err := repo.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		if repo.State != nil && repo.State.Branch != nil {
			_ = repo.State.Branch.InitializeCommits(repo)
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
		if err := repo.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		if repo.State != nil && repo.State.Branch != nil {
			_ = repo.State.Branch.InitializeCommits(repo)
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
		if err := repo.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		if repo.State != nil && repo.State.Branch != nil {
			_ = repo.State.Branch.InitializeCommits(repo)
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
