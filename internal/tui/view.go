package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const (
	maxBranchLength     = 40
	maxRepositoryLength = 30

	queuedSymbol  = "●"
	workingSymbol = "◉"
	successSymbol = "✓"
	failSymbol    = "✗"

	fetchSymbol    = "↓"
	pullSymbol     = "↓↳"
	mergeSymbol    = "↳"
	checkoutSymbol = "↱"

	pushable = "↖"
	pullable = "↘"
)

// View renders the UI
func (m *Model) View() string {
	if m.err != nil {
		return m.styles.Error.Render("Error: " + m.err.Error())
	}

	if !m.ready {
		return "Initializing..."
	}

	var content string

	if m.currentView == OverviewView {
		content = m.renderOverview()
	} else {
		content = m.renderFocus()
	}

	// Combine with status bar and help
	statusBar := m.renderStatusBar()

	if m.showHelp {
		help := m.renderHelp()
		content = lipgloss.JoinVertical(lipgloss.Left, content, help)
	}

	return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
}

// renderOverview renders the main overview with repository list
func (m *Model) renderOverview() string {
	if len(m.repositories) == 0 {
		if m.loading {
			return m.styles.List.Render("Loading repositories...")
		}
		return m.styles.List.Render("No repositories found")
	}

	// Calculate the longest repository name (capped at maxRepositoryLength)
	maxNameWidth := 0
	for _, r := range m.repositories {
		nameLen := len(r.Name)
		if nameLen > maxRepositoryLength {
			nameLen = maxRepositoryLength
		}
		if nameLen > maxNameWidth {
			maxNameWidth = nameLen
		}
	}

	// Calculate visible range based on terminal height
	visibleHeight := m.height - 5 // Reserve space for status bar
	startIdx := 0
	endIdx := len(m.repositories)

	if len(m.repositories) > visibleHeight {
		// Center cursor in view
		startIdx = m.cursor - visibleHeight/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + visibleHeight
		if endIdx > len(m.repositories) {
			endIdx = len(m.repositories)
			startIdx = endIdx - visibleHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	// Render title - stretch across full width
	titleText := fmt.Sprintf(" Matched Repositories (%d) ", len(m.repositories))
	title := m.styles.Title.Width(m.width).Render(titleText)

	// Render repositories
	var lines []string
	for i := startIdx; i < endIdx; i++ {
		r := m.repositories[i]
		line := m.renderRepositoryLine(r, i == m.cursor, maxNameWidth)
		lines = append(lines, line)
	}

	// Add scroll indicators
	if startIdx > 0 {
		lines = append([]string{m.styles.Help.Render("  ↑ more above")}, lines...)
	}
	if endIdx < len(m.repositories) {
		lines = append(lines, m.styles.Help.Render("  ↓ more below"))
	}

	// Add empty row after the list
	lines = append(lines, "")

	list := strings.Join(lines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", list)
}

// renderRepositoryLine renders a single repository line
func (m *Model) renderRepositoryLine(r *git.Repository, selected bool, maxNameWidth int) string {
	// Status indicator
	statusIcon := " "
	style := m.styles.ListItem

	switch r.WorkStatus() {
	case git.Queued:
		statusIcon = queuedSymbol
		style = m.styles.QueuedItem
	case git.Working:
		statusIcon = workingSymbol
		style = m.styles.WorkingItem
	case git.Success:
		statusIcon = successSymbol
		style = m.styles.SuccessItem
	case git.Fail:
		statusIcon = failSymbol
		style = m.styles.FailedItem
	}

	// Repository name (truncate if too long)
	name := r.Name
	if len(name) > maxRepositoryLength {
		name = name[:maxRepositoryLength-3] + "..."
	}

	// Pad the name to align branch names - add 3 spaces after the longest name
	nameWidth := len(name)
	padding := maxNameWidth - nameWidth + 3

	// Branch info
	branchName := r.State.Branch.Name
	if len(branchName) > maxBranchLength {
		branchName = branchName[:maxBranchLength-3] + "..."
	}

	branchInfo := m.styles.BranchInfo.Render(branchName)

	// Push/pull indicators
	var syncInfo string
	if r.State.Branch.Upstream != nil {
		pushables, _ := strconv.Atoi(r.State.Branch.Pushables)
		pullables, _ := strconv.Atoi(r.State.Branch.Pullables)

		if pushables > 0 {
			syncInfo += lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(fmt.Sprintf("%s%d", pushable, pushables))
		}
		if pullables > 0 {
			if syncInfo != "" {
				syncInfo += " "
			}
			syncInfo += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")).Render(fmt.Sprintf("%s%d", pullable, pullables))
		}
	}

	// Build the line with aligned branch names
	prefix := "  " // 2 spaces for indent
	if selected {
		prefix = "→ " // arrow + space for selected item
	}

	line := prefix + statusIcon + " " + name + strings.Repeat(" ", padding)
	if branchName != "" {
		line += branchInfo
	}
	if syncInfo != "" {
		line += "  " + syncInfo
	}

	if selected {
		return m.styles.SelectedItem.Render(line)
	}
	return style.Render(line)
}

// renderFocus renders the focus view with side panel
func (m *Model) renderFocus() string {
	if len(m.repositories) == 0 {
		return "No repository selected"
	}

	r := m.repositories[m.cursor]

	// Main info
	mainInfo := m.renderRepositoryInfo(r)

	// Side panel
	var panelContent string
	var panelTitle string

	switch m.sidePanel {
	case BranchPanel:
		panelTitle = "Branches"
		panelContent = m.renderBranches(r)
	case RemotePanel:
		panelTitle = "Remotes"
		panelContent = m.renderRemotes(r)
	case CommitPanel:
		panelTitle = "Commits"
		panelContent = m.renderCommits(r)
	case StatusPanel:
		panelTitle = "Status"
		panelContent = m.renderStatus(r)
	case StashPanel:
		panelTitle = "Stash"
		panelContent = m.renderStash(r)
	default:
		return mainInfo
	}

	// Style the panel
	panel := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.PanelTitle.Render(panelTitle),
		"",
		panelContent,
	)
	styledPanel := m.styles.Panel.Render(panel)

	return lipgloss.JoinHorizontal(lipgloss.Top, mainInfo, "  ", styledPanel)
}

// renderRepositoryInfo renders basic repository information
func (m *Model) renderRepositoryInfo(r *git.Repository) string {
	var lines []string

	lines = append(lines, m.styles.PanelTitle.Render("Repository: "+r.Name))
	lines = append(lines, "")
	lines = append(lines, "Branch: "+m.styles.BranchInfo.Render(r.State.Branch.Name))

	if r.State.Branch.Upstream != nil {
		lines = append(lines, "Upstream: "+m.styles.BranchInfo.Render(r.State.Branch.Upstream.Name))

		pushables, _ := strconv.Atoi(r.State.Branch.Pushables)
		pullables, _ := strconv.Atoi(r.State.Branch.Pullables)

		if pushables > 0 || pullables > 0 {
			lines = append(lines, "")
			if pushables > 0 && pullables > 0 {
				lines = append(lines, "Branch has diverged:")
				lines = append(lines, fmt.Sprintf("  %s %d commits ahead", pushable, pushables))
				lines = append(lines, fmt.Sprintf("  %s %d commits behind", pullable, pullables))
			} else if pushables > 0 {
				lines = append(lines, fmt.Sprintf("%s %d commits ahead", pushable, pushables))
			} else {
				lines = append(lines, fmt.Sprintf("%s %d commits behind", pullable, pullables))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// renderBranches renders branch list
func (m *Model) renderBranches(r *git.Repository) string {
	branches := r.Branches

	var lines []string
	for _, b := range branches {
		prefix := "  "
		if b.Name == r.State.Branch.Name {
			prefix = "→ "
		}
		lines = append(lines, prefix+b.Name)
	}

	if len(lines) == 0 {
		return "No branches"
	}

	return strings.Join(lines, "\n")
}

// renderRemotes renders remote list
func (m *Model) renderRemotes(r *git.Repository) string {
	remotes := r.Remotes

	var lines []string
	for _, remote := range remotes {
		lines = append(lines, remote.Name)
	}

	if len(lines) == 0 {
		return "No remotes"
	}

	return strings.Join(lines, "\n")
}

// renderCommits renders commit list
func (m *Model) renderCommits(r *git.Repository) string {
	if r.State.Branch == nil {
		return "No branch selected"
	}

	commits := r.State.Branch.Commits

	var lines []string
	for i, commit := range commits {
		if i >= 10 { // Limit to 10 commits
			break
		}
		hash := commit.Hash
		if len(hash) > 7 {
			hash = hash[:7]
		}
		message := commit.Message
		if len(message) > 50 {
			message = message[:47] + "..."
		}
		// Remove newlines from message
		message = strings.ReplaceAll(message, "\n", " ")
		message = strings.ReplaceAll(message, "\r", " ")

		lines = append(lines, fmt.Sprintf("%s %s",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(hash),
			message))
	}

	if len(lines) == 0 {
		return "No commits"
	}

	return strings.Join(lines, "\n")
}

// renderStatus renders git status
func (m *Model) renderStatus(r *git.Repository) string {
	var lines []string

	lines = append(lines, "On branch "+m.styles.BranchInfo.Render(r.State.Branch.Name))

	pushables, _ := strconv.Atoi(r.State.Branch.Pushables)
	pullables, _ := strconv.Atoi(r.State.Branch.Pullables)

	if r.State.Branch.Upstream == nil {
		lines = append(lines, "Not tracking a remote branch")
	} else if pushables == 0 && pullables == 0 {
		lines = append(lines, "Up to date with "+m.styles.BranchInfo.Render(r.State.Branch.Upstream.Name))
	} else {
		if pushables > 0 && pullables > 0 {
			lines = append(lines, fmt.Sprintf("Diverged from %s", r.State.Branch.Upstream.Name))
		} else if pushables > 0 {
			lines = append(lines, fmt.Sprintf("Ahead by %d commit(s)", pushables))
		} else {
			lines = append(lines, fmt.Sprintf("Behind by %d commit(s)", pullables))
		}
	}

	return strings.Join(lines, "\n")
}

// renderStash renders stash list
func (m *Model) renderStash(r *git.Repository) string {
	stashes := r.Stasheds

	var lines []string
	for _, stash := range stashes {
		lines = append(lines, fmt.Sprintf("stash@{%d}: %s %s", stash.StashID, stash.BranchName, stash.Description))
	}

	if len(lines) == 0 {
		return "No stashes"
	}

	return strings.Join(lines, "\n")
}

// renderStatusBar renders the bottom status bar
func (m *Model) renderStatusBar() string {
	modeSymbol := fetchSymbol
	statusBarStyle := m.styles.StatusBarFetch
	switch m.mode.ID {
	case PullMode:
		modeSymbol = pullSymbol
		statusBarStyle = m.styles.StatusBarPull
	case MergeMode:
		modeSymbol = mergeSymbol
		statusBarStyle = m.styles.StatusBarMerge
	case CheckoutMode:
		modeSymbol = checkoutSymbol
		statusBarStyle = m.styles.StatusBarFetch // Use fetch style for checkout
	}

	left := fmt.Sprintf(" %s %s", modeSymbol, m.mode.DisplayString)

	queuedCount := 0
	for _, r := range m.repositories {
		if r.WorkStatus() == git.Queued {
			queuedCount++
		}
	}

	center := ""
	if queuedCount > 0 {
		center = fmt.Sprintf("Queue: %d", queuedCount)
	}

	right := "TAB: lazygit | ? for help"
	if m.currentView == FocusView {
		right = "ESC: back | TAB: lazygit | ? for help"
	}

	// Calculate spacing - ensure we don't overflow the width
	totalWidth := m.width
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	centerWidth := lipgloss.Width(center)

	spacing := totalWidth - leftWidth - rightWidth - centerWidth - 2 // -2 for safety margin
	if spacing < 0 {
		spacing = 0
	}

	leftSpacing := spacing / 2
	rightSpacing := spacing - leftSpacing

	statusText := left + strings.Repeat(" ", leftSpacing) + center + strings.Repeat(" ", rightSpacing) + right

	// Ensure the status text doesn't exceed the terminal width
	if lipgloss.Width(statusText) > totalWidth {
		statusText = left + strings.Repeat(" ", max(0, totalWidth-leftWidth-rightWidth-1)) + right
	}

	return statusBarStyle.Width(totalWidth).Render(statusText)
}

// renderHelp renders the help screen
func (m *Model) renderHelp() string {
	help := `
Navigation:  ↑/k up  ↓/j down  Home/g top  End/G bottom  PgUp/PgDown page

Actions:     Space   toggle queue       Enter   start queue
             a       queue all          A       unqueue all
             m       cycle mode         Tab     open lazygit

Views:       b  branches    c  commits    r  remotes
             s  status      S  stash

Sorting:     n  by name     t  by time

Other:       ?  help        q/Ctrl+C  quit
`

	return m.styles.Help.Render(help)
}
