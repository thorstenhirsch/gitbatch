package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/job"
)

// --- Stash push prompt (like commit prompt) ---

func (m *Model) handleStashPromptKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.stashPromptActive {
		return false, nil
	}

	key := msg.String()
	switch key {
	case "ctrl+c":
		return true, tea.Quit
	case "esc":
		m.dismissStashPrompt()
		return true, nil
	case "enter":
		cmd := m.submitStash()
		return true, cmd
	case "backspace", "ctrl+h":
		runes := []rune(m.stashMessageBuffer)
		if len(runes) > 0 {
			m.stashMessageBuffer = string(runes[:len(runes)-1])
		}
		return true, nil
	case " ":
		m.stashMessageBuffer += " "
		return true, nil
	default:
		if len(msg.Runes) > 0 {
			m.stashMessageBuffer += string(msg.Runes)
		}
		return true, nil
	}
}

func (m *Model) openStashPrompt() {
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
	m.stashPromptActive = true
	m.stashPromptRepos = eligible
	m.stashMessageBuffer = ""
}

func (m *Model) dismissStashPrompt() {
	m.stashPromptActive = false
	m.stashPromptRepos = nil
	m.stashMessageBuffer = ""
}

func (m *Model) submitStash() tea.Cmd {
	msg := strings.TrimSpace(m.stashMessageBuffer)
	repos := m.stashPromptRepos
	m.dismissStashPrompt()

	if len(repos) == 0 {
		return nil
	}

	m.jobsRunning = true
	return func() tea.Msg {
		for _, repo := range repos {
			if repo == nil || repo.State == nil {
				continue
			}
			stashMsg := msg
			if stashMsg == "" && repo.State.Branch != nil {
				stashMsg = "WIP on " + repo.State.Branch.Name
			}
			repo.State.Message = "stashing.."
			repo.SetWorkStatus(git.Pending)
			j := &job.Job{
				Repository: repo,
				JobType:    job.StashJob,
				Options:    &command.StashOptions{Message: stashMsg},
			}
			if err := j.Start(); err != nil {
				repo.SetWorkStatus(git.Available)
				repo.State.Message = "stash failed: " + err.Error()
				continue
			}
		}
		return jobCompletedMsg{}
	}
}

// --- Stash action panel (pop/drop selector) ---

func (m *Model) openStashAction(action stashActionType) {
	repos := m.taggedRepositories()
	if len(repos) == 0 {
		repo := m.currentRepository()
		if repo == nil {
			return
		}
		repos = []*git.Repository{repo}
	}

	// Filter to repos with stashes
	var eligible []*git.Repository
	for _, repo := range repos {
		if repo != nil && len(repo.Stasheds) > 0 {
			eligible = append(eligible, repo)
		}
	}
	if len(eligible) == 0 {
		return
	}

	// Get stash items (common if multiple repos)
	var items []stashPanelItem
	if len(eligible) > 1 {
		items = commonStashItems(eligible)
	} else {
		items = stashItemsForRepo(eligible[0])
	}
	if len(items) == 0 {
		return
	}

	// Single stash: execute immediately without selector
	if len(items) == 1 {
		m.executeStashAction(action, eligible, items[0])
		return
	}

	// Multiple stashes: open selector panel
	m.stashAction = action
	m.stashCursor = 0
	m.stashOffset = 0
	m.sidePanel = StashActionPanel
}

func (m *Model) handleStashActionPanelKey(key string) (tea.Model, tea.Cmd) {
	items := m.stashActionPanelItems()
	count := len(items)
	if count == 0 {
		return m, nil
	}
	viewport := m.stashViewportSize(count)
	if viewport <= 0 {
		viewport = count
	}

	switch key {
	case "up", "k":
		wrapCursor(&m.stashCursor, count, -1)
	case "down", "j":
		wrapCursor(&m.stashCursor, count, 1)
	case "home", "g":
		m.stashCursor = 0
	case "end", "G":
		m.stashCursor = count - 1
	case "enter", " ", "space":
		item := items[clampIndex(m.stashCursor, count)]
		repos := m.stashActionRepos()
		m.sidePanel = NonePanel
		return m, m.executeStashActionCmd(m.stashAction, repos, item)
	}

	m.ensureStashCursorVisible(count, viewport)
	return m, nil
}

func (m *Model) stashActionRepos() []*git.Repository {
	repos := m.taggedRepositories()
	if len(repos) == 0 {
		repo := m.currentRepository()
		if repo == nil {
			return nil
		}
		repos = []*git.Repository{repo}
	}
	var result []*git.Repository
	for _, repo := range repos {
		if repo != nil && len(repo.Stasheds) > 0 {
			result = append(result, repo)
		}
	}
	return result
}

func (m *Model) executeStashAction(action stashActionType, repos []*git.Repository, item stashPanelItem) {
	m.jobsRunning = true
	go func() {
		for _, repo := range repos {
			if repo == nil || repo.State == nil {
				continue
			}
			stashRef := fmt.Sprintf("stash@{%d}", findStashID(repo, item.Description))
			var j *job.Job
			switch action {
			case stashActionPop:
				repo.State.Message = "popping stash.."
				repo.SetWorkStatus(git.Pending)
				j = &job.Job{
					Repository: repo,
					JobType:    job.StashPopJob,
					Options:    &command.StashPopOptions{StashRef: stashRef},
				}
			case stashActionDrop:
				repo.State.Message = "dropping stash.."
				repo.SetWorkStatus(git.Pending)
				j = &job.Job{
					Repository: repo,
					JobType:    job.StashDropJob,
					Options:    &command.StashDropOptions{StashRef: stashRef},
				}
			default:
				continue
			}
			if err := j.Start(); err != nil {
				repo.SetWorkStatus(git.Available)
				repo.State.Message = "stash operation failed: " + err.Error()
			}
		}
	}()
}

func (m *Model) executeStashActionCmd(action stashActionType, repos []*git.Repository, item stashPanelItem) tea.Cmd {
	if len(repos) == 0 {
		return nil
	}
	m.jobsRunning = true
	return func() tea.Msg {
		for _, repo := range repos {
			if repo == nil || repo.State == nil {
				continue
			}
			stashRef := fmt.Sprintf("stash@{%d}", findStashID(repo, item.Description))
			var j *job.Job
			switch action {
			case stashActionPop:
				repo.State.Message = "popping stash.."
				repo.SetWorkStatus(git.Pending)
				j = &job.Job{
					Repository: repo,
					JobType:    job.StashPopJob,
					Options:    &command.StashPopOptions{StashRef: stashRef},
				}
			case stashActionDrop:
				repo.State.Message = "dropping stash.."
				repo.SetWorkStatus(git.Pending)
				j = &job.Job{
					Repository: repo,
					JobType:    job.StashDropJob,
					Options:    &command.StashDropOptions{StashRef: stashRef},
				}
			default:
				continue
			}
			if err := j.Start(); err != nil {
				repo.SetWorkStatus(git.Available)
				repo.State.Message = "stash operation failed: " + err.Error()
			}
		}
		return jobCompletedMsg{}
	}
}

// findStashID looks up the stash ID in a repo by matching the description.
// Falls back to 0 if not found.
func findStashID(repo *git.Repository, description string) int {
	for _, s := range repo.Stasheds {
		if s != nil && s.Description == description {
			return s.StashID
		}
	}
	return 0
}

func (m *Model) stashViewportSize(count int) int {
	_, maxLines := m.popupDimensions()
	viewport := maxLines - 4 // header lines
	if viewport > count {
		viewport = count
	}
	if viewport < 1 {
		viewport = 1
	}
	return viewport
}

func (m *Model) ensureStashCursorVisible(count, viewport int) {
	ensureCursorVisible(&m.stashCursor, &m.stashOffset, count, viewport)
}
