package tui

import (
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
		m.sidePanel = BranchPanel
		m.currentView = FocusView

	case "c":
		m.sidePanel = CommitPanel
		m.currentView = FocusView

	case "r":
		m.sidePanel = RemotePanel
		m.currentView = FocusView

	case "s":
		m.sidePanel = StatusPanel
		m.currentView = FocusView

	case "S":
		m.sidePanel = StashPanel
		m.currentView = FocusView

	case "n":
		m.sortByName()

	case "t":
		m.sortByTime()
	}

	return m, nil
}

// handleFocusKeys processes keys in focus mode
func (m *Model) handleFocusKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		// Return to overview
		m.currentView = OverviewView
		m.sidePanel = NonePanel
	}

	return m, nil
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
