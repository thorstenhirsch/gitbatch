package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const (
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

	repoColPrefixWidth = 4 // cursor + space + status + space
)

type columnWidths struct {
	repo      int
	branch    int
	commitMsg int
}

const (
	maxRepoDisplayWidth   = 40
	maxBranchDisplayWidth = 40
	commitColumnMinWidth  = 10
)

func calculateColumnWidths(totalWidth int, repos []*git.Repository) columnWidths {
	if totalWidth <= 0 {
		return columnWidths{}
	}

	repoNameWidth := clampInt(maxRepoNameLength(repos), 0, maxRepoDisplayWidth) + 5
	branchNameWidth := clampInt(maxBranchNameLength(repos), 0, maxBranchDisplayWidth) + 5

	widths := columnWidths{
		repo:      repoColPrefixWidth + repoNameWidth,
		branch:    1 + branchNameWidth,
		commitMsg: commitColumnMinWidth,
	}

	borders := 4 // │repo│branch│commit│
	if totalWidth <= borders {
		safeRepo := repoColPrefixWidth
		if safeRepo > totalWidth {
			safeRepo = totalWidth
		}
		if safeRepo < 0 {
			safeRepo = 0
		}
		return columnWidths{repo: safeRepo, branch: 0, commitMsg: 0}
	}

	available := totalWidth - borders
	if available < 0 {
		available = 0
	}

	sum := widths.repo + widths.branch + widths.commitMsg
	if sum > available {
		deficit := sum - available
		reduceWidth(&widths.commitMsg, &deficit, commitColumnMinWidth)
		reduceWidth(&widths.branch, &deficit, 1)
		reduceWidth(&widths.repo, &deficit, repoColPrefixWidth)
		if deficit > 0 {
			reduceWidth(&widths.commitMsg, &deficit, 1)
		}
		if deficit > 0 {
			reduceWidth(&widths.branch, &deficit, 0)
		}
		if deficit > 0 {
			reduceWidth(&widths.repo, &deficit, 0)
		}
	}

	if widths.repo < 0 {
		widths.repo = 0
	}
	if widths.branch < 0 {
		widths.branch = 0
	}
	if widths.commitMsg < 0 {
		widths.commitMsg = 0
	}

	used := widths.repo + widths.branch + widths.commitMsg
	if used > available {
		excess := used - available
		reduceWidth(&widths.commitMsg, &excess, 0)
		reduceWidth(&widths.branch, &excess, 0)
		reduceWidth(&widths.repo, &excess, 0)
		used = widths.repo + widths.branch + widths.commitMsg
	}

	remaining := available - used
	if remaining > 0 {
		widths.commitMsg += remaining
	}

	return widths
}

func maxRepoNameLength(repos []*git.Repository) int {
	maxLen := 0
	for _, r := range repos {
		if r == nil {
			continue
		}
		if length := lipgloss.Width(r.Name); length > maxLen {
			maxLen = length
		}
	}
	return maxLen
}

func maxBranchNameLength(repos []*git.Repository) int {
	maxLen := 0
	for _, r := range repos {
		if r == nil || r.State == nil || r.State.Branch == nil {
			continue
		}
		if length := lipgloss.Width(r.State.Branch.Name); length > maxLen {
			maxLen = length
		}
	}
	return maxLen
}

func branchContent(r *git.Repository) string {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return ""
	}
	return r.State.Branch.Name + syncSuffix(r.State.Branch)
}

func syncSuffix(branch *git.Branch) string {
	if branch == nil || branch.Upstream == nil {
		return ""
	}
	pushables, _ := strconv.Atoi(branch.Pushables)
	pullables, _ := strconv.Atoi(branch.Pullables)
	var parts []string
	if pushables > 0 {
		parts = append(parts, fmt.Sprintf("%s%d", pushable, pushables))
	}
	if pullables > 0 {
		parts = append(parts, fmt.Sprintf("%s%d", pullable, pullables))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func reduceWidth(current *int, deficit *int, min int) {
	if deficit == nil || current == nil {
		return
	}
	if *deficit <= 0 {
		return
	}
	if min < 0 {
		min = 0
	}
	reducible := *current - min
	if reducible <= 0 {
		return
	}
	delta := reducible
	if delta > *deficit {
		delta = *deficit
	}
	*current -= delta
	*deficit -= delta
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// renderTableBorder renders a horizontal border for the table
// Example: "┌────────────┬────────────┬────────────┐"
func (m *Model) renderTableBorder(colWidths columnWidths, borderType string) string {
	var left, mid, right, horiz string
	switch borderType {
	case "top":
		left, mid, right, horiz = "┌", "┬", "┐", "─"
	case "bottom":
		left, mid, right, horiz = "└", "┴", "┘", "─"
	default:
		left, mid, right, horiz = "├", "┼", "┤", "─"
	}

	border := left +
		strings.Repeat(horiz, colWidths.repo) + mid +
		strings.Repeat(horiz, colWidths.branch) + mid +
		strings.Repeat(horiz, colWidths.commitMsg) +
		right

	return m.styles.TableBorder.Render(border)
}

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

	// Status bar is always at the bottom
	statusBar := m.renderStatusBar()

	if m.showHelp {
		help := m.renderHelp()
		content = lipgloss.JoinVertical(lipgloss.Left, content, help)
	}

	// Fill remaining space and ensure status bar is at bottom
	contentHeight := lipgloss.Height(content)
	statusBarHeight := 1
	remainingHeight := m.height - contentHeight - statusBarHeight

	if remainingHeight > 0 {
		content += strings.Repeat("\n", remainingHeight)
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

	// Calculate visible range based on terminal height
	// Reserve space for: title (1) + top border (1) + bottom border (1) + status bar (1)
	visibleHeight := m.height - 4
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

	// Compute column widths based on content and available width
	colWidths := calculateColumnWidths(m.width, m.repositories)

	// Render title - stretch across full width
	titleText := fmt.Sprintf(" Matched Repositories (%d) ", len(m.repositories))
	title := m.styles.Title.Width(m.width).Render(titleText)

	// Top border for table
	topBorder := m.renderTableBorder(colWidths, "top")

	// Render repositories
	var lines []string
	for i := startIdx; i < endIdx; i++ {
		r := m.repositories[i]
		line := m.renderRepositoryLine(r, i == m.cursor, colWidths)
		lines = append(lines, line)
	}

	// Add scroll indicators
	if startIdx > 0 {
		scrollUp := m.styles.Help.Render("  ↑ more above")
		lines = append([]string{scrollUp}, lines...)
	}
	if endIdx < len(m.repositories) {
		scrollDown := m.styles.Help.Render("  ↓ more below")
		lines = append(lines, scrollDown)
	}

	// Fill remaining rows with empty table rows to stretch to full height
	currentRowCount := len(lines)
	for currentRowCount < visibleHeight {
		border := m.styles.TableBorder.Render("│")
		emptyRepoCol := strings.Repeat(" ", colWidths.repo)
		emptyBranchCol := strings.Repeat(" ", colWidths.branch)
		emptyCommitCol := strings.Repeat(" ", colWidths.commitMsg)
		emptyRow := border + emptyRepoCol + border + emptyBranchCol + border + emptyCommitCol + border
		lines = append(lines, emptyRow)
		currentRowCount++
	}

	// Bottom border for table
	bottomBorder := m.renderTableBorder(colWidths, "bottom")

	list := strings.Join(lines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, title, topBorder, list, bottomBorder)
}

// renderRepositoryLine renders a single repository line as a table row
// Table format: │cursor status repo-name    │ branch-name │ commit tags/message │
// Example:      │→ ●   example-repo         │  main       │ [v1.0.0] add feature │
func (m *Model) renderRepositoryLine(r *git.Repository, selected bool, colWidths columnWidths) string {
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

	cursor := " "
	if selected {
		cursor = "→"
	}

	repoNameWidth := colWidths.repo - repoColPrefixWidth
	if repoNameWidth < 0 {
		repoNameWidth = 0
	}
	repoName := truncateString(r.Name, repoNameWidth)
	repoColumn := fmt.Sprintf("%s %s %-*s", cursor, statusIcon, repoNameWidth, repoName)

	branchContentWidth := colWidths.branch - 1
	if branchContentWidth < 0 {
		branchContentWidth = 0
	}
	branchContent := truncateString(branchContent(r), branchContentWidth)
	branchColumn := fmt.Sprintf("%-*s", colWidths.branch, " "+branchContent)

	commitMsg, commitHash := commitSummary(r)
	tags := collectTags(r, commitHash)
	var tagsText string
	if len(tags) > 0 {
		tagsText = "[" + strings.Join(tags, ", ") + "]"
	}
	commitParts := make([]string, 0, 2)
	if tagsText != "" {
		commitParts = append(commitParts, tagsText)
	}
	if commitMsg != "" {
		commitParts = append(commitParts, commitMsg)
	}
	commitContent := strings.Join(commitParts, " ")
	commitContentWidth := colWidths.commitMsg - 1
	if commitContentWidth < 0 {
		commitContentWidth = 0
	}
	commitContent = truncateString(commitContent, commitContentWidth)
	commitColumn := fmt.Sprintf("%-*s", colWidths.commitMsg, " "+commitContent)

	var styledRepoCol, styledBranchCol, styledCommitCol string
	if selected {
		styledRepoCol = m.styles.SelectedItem.Render(repoColumn)
		styledBranchCol = m.styles.SelectedItem.Render(branchColumn)
		styledCommitCol = m.styles.SelectedItem.Render(commitColumn)
	} else {
		styledRepoCol = style.Render(repoColumn)
		styledBranchCol = style.Render(branchColumn)
		styledCommitCol = style.Render(commitColumn)
	}

	border := m.styles.TableBorder.Render("│")
	return border + styledRepoCol + border + styledBranchCol + border + styledCommitCol + border
}

func commitSummary(r *git.Repository) (string, plumbing.Hash) {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return "", plumbing.Hash{}
	}

	branch := r.State.Branch
	if branch.State != nil && branch.State.Commit != nil {
		commitState := branch.State.Commit
		message := commitState.Message
		if commitState.C != nil {
			return firstLine(commitState.C.Message), commitState.C.Hash
		}
		if commitState.Hash != "" {
			return firstLine(message), plumbing.NewHash(commitState.Hash)
		}
		return firstLine(message), plumbing.Hash{}
	}

	if branch.Reference != nil {
		if commitObj, err := r.Repo.CommitObject(branch.Reference.Hash()); err == nil {
			return firstLine(commitObj.Message), commitObj.Hash
		}
	}

	return "", plumbing.Hash{}
}

func collectTags(r *git.Repository, commitHash plumbing.Hash) []string {
	if r == nil || commitHash.IsZero() {
		return nil
	}

	iter, err := r.Repo.Tags()
	if err != nil {
		return nil
	}
	defer iter.Close()

	var tags []string
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref == nil {
			return nil
		}
		if ref.Type() != plumbing.HashReference {
			return nil
		}
		hash := ref.Hash()
		if tagObj, err := r.Repo.TagObject(hash); err == nil {
			if tagObj.Target == commitHash {
				tags = append(tags, tagObj.Name)
			}
			return nil
		}
		if hash == commitHash {
			tags = append(tags, ref.Name().Short())
		}
		return nil
	})

	if len(tags) > 1 {
		sort.Strings(tags)
	}

	return tags
}

func firstLine(message string) string {
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		message = message[:idx]
	}
	return strings.TrimSpace(message)
}

// truncateString truncates a string to the specified length, adding "..." if needed
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
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
		if m.hasMultipleTagged() {
			panelTitle = "Common Branches"
		} else {
			panelTitle = "Branches"
		}
		panelContent = m.renderBranches(r)
	case RemotePanel:
		if m.hasMultipleTagged() {
			panelTitle = "Common Remote Branches"
		} else {
			panelTitle = "Remotes"
		}
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

	panelStyle := m.styles.Panel
	if (m.sidePanel == BranchPanel || m.sidePanel == RemotePanel) && m.hasMultipleTagged() {
		panelStyle = panelStyle.Copy().BorderForeground(tagHighlightColor)
	}

	styledPanel := panelStyle.Render(panel)

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
	items := m.branchPanelItems()
	if len(items) == 0 {
		if m.hasMultipleTagged() {
			return "No common branches"
		}
		return "No branches"
	}

	cursor := m.branchCursor
	if cursor < 0 || cursor >= len(items) {
		cursor = 0
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s checkout  %s delete",
		m.styles.KeyBinding.Render("[enter]"),
		m.styles.KeyBinding.Render("[d]"),
	))
	lines = append(lines, "")

	for i, item := range items {
		prefix := "  "
		if item.IsCurrent {
			prefix = "→ "
		}
		line := prefix + item.Name
		if i == cursor {
			lines = append(lines, m.styles.SelectedItem.Render(line))
		} else {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// renderRemotes renders remote list
func (m *Model) renderRemotes(r *git.Repository) string {
	items := m.remotePanelItems()
	if len(items) == 0 {
		if m.hasMultipleTagged() {
			return "No common remote branches"
		}
		return "No remote branches"
	}

	cursor := m.remoteBranchCursor
	if cursor < 0 || cursor >= len(items) {
		cursor = 0
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s checkout  %s delete",
		m.styles.KeyBinding.Render("[enter]"),
		m.styles.KeyBinding.Render("[d]"),
	))
	lines = append(lines, "")

	for i, item := range items {
		line := fmt.Sprintf("%s %s", item.RemoteName, item.BranchName)
		if i == cursor {
			lines = append(lines, m.styles.SelectedItem.Render(line))
		} else {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// renderCommits renders commit list
func (m *Model) renderCommits(r *git.Repository) string {
	if m.hasMultipleTagged() {
		return "Commit view unavailable when multiple repositories are tagged"
	}

	if r == nil || r.State == nil || r.State.Branch == nil {
		return "No branch selected"
	}

	commits := r.State.Branch.Commits

	count := len(commits)
	if count == 0 {
		return "No commits"
	}

	viewport := m.commitViewportSize()
	if viewport > count {
		viewport = count
	}
	m.ensureCommitCursorVisible(count, viewport)

	start := m.commitOffset
	if start < 0 {
		start = 0
	}
	maxStart := count - viewport
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end := start + viewport
	if end > count {
		end = count
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s checkout  %s soft reset  %s mixed reset  %s hard reset",
		m.styles.KeyBinding.Render("[enter]"),
		m.styles.KeyBinding.Render("[s]"),
		m.styles.KeyBinding.Render("[m]"),
		m.styles.KeyBinding.Render("[h]"),
	))
	lines = append(lines, "")

	if start > 0 {
		lines = append(lines, m.styles.Help.Render("  ↑ more above"))
	}

	for i := start; i < end; i++ {
		commit := commits[i]
		label := ""
		hash := "(none)"
		message := ""
		if commit != nil {
			switch commit.CommitType {
			case git.LocalCommit:
				label = "[local] "
			case git.RemoteCommit:
				label = "[remote] "
			}
			hash = shortHash(commit.Hash)
			message = firstLine(commit.Message)
			if len(message) > 60 {
				message = truncateString(message, 60)
			}
			message = strings.ReplaceAll(message, "\n", " ")
			message = strings.ReplaceAll(message, "\r", " ")
		}
		line := fmt.Sprintf("%s%s %s", label, hash, message)
		if i == m.commitCursor {
			lines = append(lines, m.styles.SelectedItem.Render(line))
		} else {
			lines = append(lines, line)
		}
	}

	if end < count {
		lines = append(lines, m.styles.Help.Render("  ↓ more below"))
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
Navigation:  ↑/k up   g/Home top        ↓/j down G/End bottom
             PgUp/Ctrl+B page up        PgDn/Ctrl+F page down
             Ctrl+U half page up        Ctrl+D half page down

Actions:     Space   toggle queue       Enter   start queue
             a       queue all          A       unqueue all
             m       cycle mode         Tab     open lazygit

Views:       b  branches    c  commits    r  remotes
             s  status      S  stash      ESC back (from views)

Sorting:     n  by name     t  by time

Other:       ?  help        q/Ctrl+C  quit
`

	return m.styles.Help.Render(help)
}
