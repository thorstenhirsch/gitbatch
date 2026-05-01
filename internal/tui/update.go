package tui

import (
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// focusRefreshDebounce caps how often a terminal focus-gain triggers a
// working-tree refresh across all repos. Rapid alt-tabs should not cause
// repeated fan-outs.
const focusRefreshDebounce = 5 * time.Second

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
		m.applyRepositorySort()
		if m.cursor >= m.overviewRowCount() {
			m.cursor = m.findLastNavigableIndex()
		} else {
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
		if m.sortMode == repositorySortByTime {
			m.applyRepositorySort()
		}
		if m.worktreeMode {
			m.cursor = m.closestSelectableIndex(m.cursor, 1)
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

	case tea.FocusMsg:
		return m, m.focusRefreshCmd(false)

	case tea.BlurMsg:
		return m, nil
	}

	return m, nil
}

// focusRefreshCmd returns a Cmd that fans out a working-tree refresh to every
// loaded repo when the user returns to gitbatch (e.g. alt-tab from an editor).
// The debounce check runs synchronously on the Update goroutine; the actual
// fan-out runs in the returned Cmd's goroutine so user input and rendering are
// not blocked even across dozens of repos.
//
// If force is true the debounce is bypassed (used by the manual refresh key).
// In-flight repos are skipped; the git semaphore inside the refresh pipeline
// caps concurrency globally.
func (m *Model) focusRefreshCmd(force bool) tea.Cmd {
	m.updateMu.Lock()
	if !force && !m.lastFocusRefresh.IsZero() && time.Since(m.lastFocusRefresh) < focusRefreshDebounce {
		m.updateMu.Unlock()
		return nil
	}
	m.lastFocusRefresh = time.Now()
	m.updateMu.Unlock()

	repos := make([]*git.Repository, len(m.repositories))
	copy(repos, m.repositories)

	return func() tea.Msg {
		for _, r := range repos {
			if r == nil || r.WorkStatus().InFlight() {
				continue
			}
			command.RequestExternalRefresh(r)
		}
		return nil
	}
}

func (m *Model) handleLazygitClosed(msg lazygitClosedMsg) (tea.Model, tea.Cmd) {
	repo := msg.repo
	if repo.RefreshModTime().After(msg.originalModTime) {
		// The TAB handler set Working as a lock while lazygit ran. Clear it so
		// RequestExternalRefresh (which skips InFlight repos) can actually run.
		repo.SetWorkStatusSilent(git.Available)
		command.RequestExternalRefresh(repo)
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

// addRepository inserts r into m.repositories and registers its event listeners.
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

	if m.watcher != nil {
		m.watcher.Register(r)
	}

	m.repositories = rs
	if m.sortMode == repositorySortByTime {
		m.applyRepositorySort()
	}
}

func (m *Model) currentRepository() *git.Repository {
	row, ok := m.currentOverviewRow()
	if !ok {
		return nil
	}
	return row.repository()
}

func (m *Model) currentWorktreeCommandRepository() *git.Repository {
	row, ok := m.currentOverviewRow()
	if !ok {
		return nil
	}
	if repo := row.actionRepository(); repo != nil {
		return repo
	}
	return row.repository()
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
	rows := m.overviewRows()
	count := len(rows)
	if count == 0 {
		return 0
	}
	if start < 0 {
		return m.lastSelectableIndex()
	}
	if start >= count {
		return m.firstSelectableIndex()
	}
	return m.closestSelectableIndex(start, 1)
}

func (m *Model) findLastNavigableIndex() int {
	if m.overviewRowCount() > 0 {
		return m.lastSelectableIndex()
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
	if repo.IsLinkedWorktree() {
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
