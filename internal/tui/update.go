package tui

import (
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const (
	// TargetFPS is the maximum frames per second for the UI.
	TargetFPS = 60
	// FrameDuration is the minimum time between frames.
	FrameDuration = time.Second / TargetFPS

	// SpinnerFPS is the update rate for the activity spinner.
	SpinnerFPS = 15
	// SpinnerDuration is the time between spinner updates.
	SpinnerDuration = time.Second / SpinnerFPS

	// JobCheckInterval is how often the model checks for running jobs.
	JobCheckInterval = 100 * time.Millisecond
)

// shouldThrottleCheck returns true (and advances lastCheck) when enough time has
// passed since the last check, false otherwise. Protected by m.updateMu.
func (m *Model) shouldThrottleCheck(lastCheck *time.Time, interval time.Duration) bool {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	now := time.Now()
	if now.Sub(*lastCheck) < interval {
		return false
	}
	*lastCheck = now
	return true
}

// listenRepositoryUpdatesCmd blocks until a repository signals an update, then
// returns a repositoryStateChangedMsg. It drains bursts and throttles to TargetFPS.
func (m *Model) listenRepositoryUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		<-m.repositoryUpdateCh
		for len(m.repositoryUpdateCh) > 0 {
			<-m.repositoryUpdateCh
		}
		m.updateMu.Lock()
		elapsed := time.Since(m.lastUpdateCheck)
		if elapsed < FrameDuration {
			wait := FrameDuration - elapsed
			m.updateMu.Unlock()
			time.Sleep(wait)
			m.updateMu.Lock()
		}
		m.lastUpdateCheck = time.Now()
		m.updateMu.Unlock()
		return repositoryStateChangedMsg{}
	}
}

// enqueueRepositoryUpdate signals that a repository has changed state.
// Non-blocking: if the buffer is full the signal is dropped (the existing one suffices).
func (m *Model) enqueueRepositoryUpdate() {
	select {
	case m.repositoryUpdateCh <- struct{}{}:
	default:
	}
}

// Update is the main Bubbletea message handler.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, m.maybeStartInitialStateEvaluation(nil)

	case repositoriesLoadedMsg:
		for _, repo := range msg.repos {
			if repo != nil {
				m.addRepository(repo)
				if repo.State != nil {
					repo.State.Message = "waiting"
				}
				repo.SetWorkStatus(git.Pending)
			}
		}
		if m.cursor >= len(m.repositories) {
			m.cursor = len(m.repositories) - 1
			m.cursor = m.findNextReadyIndex(m.cursor)
		}
		m.loading = false
		select {
		case loadProgressCh <- 0:
		default:
		}
		return m, m.maybeStartInitialStateEvaluation(nil)

	case repositoryStateChangedMsg:
		// Throttle O(n) job check to avoid starvation on large repo lists.
		if m.shouldThrottleCheck(&m.lastJobCheck, 100*time.Millisecond) {
			m.updateJobsRunningFlag()
		}
		return m, tea.Batch(m.ensureTicking(), m.listenRepositoryUpdatesCmd())

	case repositoriesWaitingMsg:
		return m, m.ensureTicking()

	case lazygitClosedMsg:
		return m.handleLazygitClosed(msg)

	case jobCompletedMsg:
		if m.jobsRunning || m.loading {
			m.advanceSpinner()
		}
		if m.shouldThrottleCheck(&m.lastJobCheck, JobCheckInterval) {
			m.updateJobsRunningFlag()
		}
		if m.jobsRunning || m.loading {
			return m, tickCmd()
		}
		m.tickRunning = false
		return m, nil

	case repoActionResultMsg:
		if msg.closePanel {
			m.activatePanel(NonePanel)
			m.clearSuccessFormatting()
			return m, nil
		}
		m.ensureSelectionWithinBounds(msg.panel)
		return m, nil

	case repoLoadProgressMsg:
		m.loadedCount = msg.count
		if m.loading {
			return m, listenLoadProgressCmd()
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}

	return m, nil
}

func (m *Model) handleLazygitClosed(msg lazygitClosedMsg) (tea.Model, tea.Cmd) {
	repo := msg.repo
	if repo.RefreshModTime().After(msg.originalModTime) {
		repo.State.Message = "waiting"
		repo.SetWorkStatus(git.Pending)
		repo.NotifyRepositoryUpdated()
		_ = command.ScheduleRepositoryRefresh(repo, nil)
		m.jobsRunning = true
		return m, m.ensureTicking()
	}
	*repo.State = msg.originalState
	repo.NotifyRepositoryUpdated()
	if m.updateJobsRunningFlag() {
		return m, m.ensureTicking()
	}
	return m, nil
}

// addRepository inserts r into m.repositories in alphabetical order and
// registers its event listeners.
func (m *Model) addRepository(r *git.Repository) {
	rs := m.repositories
	index := sort.Search(len(rs), func(i int) bool {
		return git.CompareNamesInsensitive(r.Name, rs[i].Name) < 0
	})
	rs = append(rs, &git.Repository{})
	copy(rs[index+1:], rs[index:])
	rs[index] = r

	r.On(git.RepositoryUpdated, func(_ *git.RepositoryEvent) error {
		m.enqueueRepositoryUpdate()
		return nil
	})
	r.On(git.BranchUpdated, func(_ *git.RepositoryEvent) error {
		m.enqueueRepositoryUpdate()
		return nil
	})

	m.repositories = rs
}

func (m *Model) currentRepository() *git.Repository {
	if len(m.repositories) == 0 || m.cursor < 0 || m.cursor >= len(m.repositories) {
		return nil
	}
	return m.repositories[m.cursor]
}

func (m *Model) updateJobsRunningFlag() bool {
	for _, r := range m.repositories {
		s := r.WorkStatus()
		if s == git.Working || s == git.Pending {
			m.jobsRunning = true
			return true
		}
	}
	m.jobsRunning = false
	return false
}

func (m *Model) advanceSpinner() {
	if len(spinnerFrames) > 0 {
		m.spinnerIndex = (m.spinnerIndex + 1) % len(spinnerFrames)
	}
}

func (m *Model) findNextReadyIndex(start int) int {
	count := len(m.repositories)
	if count == 0 {
		return 0
	}
	if start < 0 {
		return count - 1
	}
	if start >= count {
		return 0
	}
	return start
}

func (m *Model) findLastNavigableIndex() int {
	if count := len(m.repositories); count > 0 {
		return count - 1
	}
	return 0
}

// --- Repository state helpers ---

func repoHasActiveJob(status git.WorkStatus) bool {
	return status == git.Pending || status == git.Queued || status == git.Working
}

func repoIsDirty(repo *git.Repository) bool {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return false
	}
	return !repo.State.Branch.Clean
}

func repoHasLocalChanges(repo *git.Repository) bool {
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return false
	}
	return repo.State.Branch.HasLocalChanges
}

func repoIsActionable(repo *git.Repository) bool {
	if repo == nil {
		return false
	}
	status := repo.WorkStatus()
	if status == git.Fail {
		// Allow retry on a clean-message fail (preserves fail visualization).
		if repo.State == nil || repo.State.Message != "" {
			return false
		}
	} else if !status.Ready {
		return false
	}
	if repo.State == nil || repo.State.Branch == nil {
		return false
	}
	return repo.State.Branch.Clean
}

func defaultRemoteName(repo *git.Repository) string {
	if repo != nil && repo.State != nil && repo.State.Remote != nil && repo.State.Remote.Name != "" {
		return repo.State.Remote.Name
	}
	return "origin"
}
