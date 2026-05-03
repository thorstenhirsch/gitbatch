package tui

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const (
	queuedSymbol       = "●"
	successSymbol      = "✓"
	failSymbol         = "✗"
	dirtySymbol        = "⚠"
	localChangesSymbol = "~"

	pullSymbol    = "↓"
	mergeSymbol   = "↣"
	rebaseSymbol  = "↯"
	pushSymbol    = "↑"
	waitingSymbol = "…"

	pushable = "↖"
	pullable = "↘"

	repoColPrefixWidth = 4 // cursor + space + status + space

	minTerminalWidth  = 60
	minTerminalHeight = 8
)

var commonPanelBorderColor = lipgloss.AdaptiveColor{Light: "#FB8C00", Dark: "#FFB74D"}

var (
	worktreeDiffAdditionStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#66BB6A"})
	worktreeDiffDeletionStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#EF5350"})
	worktreeDiffInsertionsRE = regexp.MustCompile(`(\d+)\s+insertions?\(\+\)`)
	worktreeDiffDeletionsRE  = regexp.MustCompile(`(\d+)\s+deletions?\(-\)`)
)

const (
	maxRepoDisplayWidth   = 40
	maxBranchDisplayWidth = 40
	commitColumnMinWidth  = 10
)

const panelHorizontalFrame = 4

// getColumnWidths returns cached column widths, recalculating only when necessary
func (m *Model) getColumnWidths() columnWidths {
	// Check if we need to recalculate
	if m.cachedWidth != m.width || m.cachedRepoCount != len(m.repositories) {
		m.cachedColWidths = calculateColumnWidths(m.width, m.repositories)
		m.cachedWidth = m.width
		m.cachedRepoCount = len(m.repositories)
	}
	return m.cachedColWidths
}

func (m *Model) popupDimensions() (popupWidth, maxContentLines int) {
	popupWidth = m.width * 70 / 100
	if popupWidth > 80 {
		popupWidth = 80
	}
	if popupWidth < 40 && m.width >= 40 {
		popupWidth = 40
	}
	if m.width-4 < popupWidth {
		popupWidth = m.width - 4
	}
	if popupWidth < panelHorizontalFrame+1 {
		popupWidth = panelHorizontalFrame + 1
	}

	maxContentLines = (m.height * 70 / 100) - 2
	if maxContentLines < 5 {
		maxContentLines = 5
	}
	return
}

func calculateColumnWidths(totalWidth int, repos []*git.Repository) columnWidths {
	available := totalWidth - 4 // account for table borders
	if available <= 0 {
		return columnWidths{}
	}

	repoMin := repoColPrefixWidth + 1
	branchMin := 1
	commitMin := commitColumnMinWidth

	repoWidth := repoMin
	branchWidth := branchMin
	commitWidth := commitMin

	extra := available - (repoWidth + branchWidth + commitWidth)
	if extra < 0 {
		extra = 0
	}

	repoTarget := repoColPrefixWidth + clampInt(maxRepoNameLength(repos), 0, maxRepoDisplayWidth) + 5
	if repoTarget < repoWidth {
		repoTarget = repoWidth
	}
	growRepo := clampInt(repoTarget-repoWidth, 0, extra)
	repoWidth += growRepo
	extra -= growRepo

	branchTarget := 1 + clampInt(maxBranchNameLength(repos), 0, maxBranchDisplayWidth) + 6
	if branchTarget < branchWidth {
		branchTarget = branchWidth
	}
	growBranch := clampInt(branchTarget-branchWidth, 0, extra)
	branchWidth += growBranch
	extra -= growBranch

	commitWidth += extra

	return columnWidths{
		repo:      repoWidth,
		branch:    branchWidth,
		commitMsg: commitWidth,
	}
}

// repoDisplayName returns the repo name with a stash indicator suffix if stashes exist.
// e.g. "myrepo {2}" for 3 stashes (highest index = 2), or just "myrepo" if none.
func repoDisplayName(r *git.Repository) string {
	if r == nil {
		return ""
	}
	if len(r.Stasheds) == 0 {
		return r.Name
	}
	return fmt.Sprintf("%s {%d}", r.Name, len(r.Stasheds)-1)
}

func maxRepoNameLength(repos []*git.Repository) int {
	maxLen := 0
	for _, r := range repos {
		if r == nil {
			continue
		}
		if length := lipgloss.Width(repoDisplayName(r)); length > maxLen {
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

func renderRepoColumnBody(left string, width int, right string, rightWidth int) string {
	if width <= 0 {
		return ""
	}
	if right == "" || rightWidth <= 0 || width <= rightWidth+1 {
		return fmt.Sprintf("%-*s", width, truncateString(left, width))
	}

	left = truncateString(left, width-rightWidth-1)
	padding := width - lipgloss.Width(left) - rightWidth
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + right
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
func (m *Model) renderTableBorder(colWidths columnWidths, borderType string, label string) string {
	var left, mid, right, horiz string
	switch borderType {
	case "top":
		left, mid, right, horiz = "┌", "┬", "┐", "─"
	case "bottom":
		left, mid, right, horiz = "└", "┴", "┘", "─"
	default:
		left, mid, right, horiz = "├", "┼", "┤", "─"
	}

	repoSeg := borderSegmentWithLeftLabel(colWidths.repo, horiz, label)
	branchSeg := strings.Repeat(horiz, colWidths.branch)
	commitSeg := strings.Repeat(horiz, colWidths.commitMsg)

	border := left + repoSeg + mid + branchSeg + mid + commitSeg + right

	return m.styles.TableBorder.Render(border)
}

func borderSegmentWithLeftLabel(width int, horiz, label string) string {
	if width <= 0 {
		return ""
	}
	if label == "" {
		return strings.Repeat(horiz, width)
	}
	decorated := "(" + label + ")"
	fitted := fitStringToWidth(decorated, width-3)
	if fitted == "" {
		return strings.Repeat(horiz, width)
	}
	labelWidth := lipgloss.Width(fitted)
	padding := width - labelWidth
	if padding < 3 {
		padding = 3
	}
	leftPad := 3
	rightPad := padding - leftPad
	if rightPad < 0 {
		rightPad = 0
	}
	if leftPad+labelWidth+rightPad < width {
		rightPad = width - (leftPad + labelWidth)
	}
	if leftPad+labelWidth+rightPad > width {
		// trim to fit exactly if rounding issues
		diff := leftPad + labelWidth + rightPad - width
		if rightPad >= diff {
			rightPad -= diff
		} else if leftPad >= diff {
			leftPad -= diff
		} else {
			fitted = fitStringToWidth(fitted, width-leftPad-rightPad)
			labelWidth = lipgloss.Width(fitted)
			if leftPad+labelWidth+rightPad > width {
				rightPad = width - (leftPad + labelWidth)
				if rightPad < 0 {
					rightPad = 0
				}
			}
		}
	}
	return strings.Repeat(horiz, leftPad) + fitted + strings.Repeat(horiz, rightPad)
}

func fitStringToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	var b strings.Builder
	current := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if current+rw > width {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	return b.String()
}

func padToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	trimmed := fitStringToWidth(s, width)
	lineWidth := lipgloss.Width(trimmed)
	if lineWidth < width {
		trimmed += strings.Repeat(" ", width-lineWidth)
	}
	return trimmed
}

func clampLines(lines []string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	return lines[:maxLines]
}

// renderLoadingScreen renders a centered loading screen with a progress bar.
// It is shown during the initial load when more than loadingScreenThreshold
// repositories are being discovered.
func (m *Model) renderLoadingScreen() string {
	total := len(m.directories)
	loaded := m.loadedCount

	spinner := spinnerFrames[m.spinnerIndex%len(spinnerFrames)]

	// Progress bar
	barWidth := 30
	if m.width > 60 {
		barWidth = m.width / 3
		if barWidth > 40 {
			barWidth = 40
		}
	}

	filled := 0
	if total > 0 {
		filled = loaded * barWidth / total
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := "[" +
		strings.Repeat("█", filled) +
		strings.Repeat("░", barWidth-filled) +
		"]"

	counterLine := fmt.Sprintf("%d / %d repositories", loaded, total)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
		Background(lipgloss.AdaptiveColor{Light: "#5E35B1", Dark: "#7E57C2"}).
		Padding(0, 2)

	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#5E35B1", Dark: "#9575CD"})

	counterStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#424242", Dark: "#BDBDBD"})

	spinnerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#5E35B1", Dark: "#9575CD"}).
		Bold(true)

	box := lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render("gitbatch"),
		"",
		spinnerStyle.Render(spinner)+" Loading repositories...",
		"",
		barStyle.Render(bar),
		counterStyle.Render(counterLine),
	)

	boxStyled := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#7E57C2", Dark: "#9575CD"}).
		Padding(1, 3).
		Render(box)

	return lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, boxStyled,
		lipgloss.WithWhitespaceChars(" "),
	)
}

// View renders the UI
func (m *Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.terminalTooSmall() {
		msg := fmt.Sprintf(
			"terminal is too small\nminimum size: width %d, height %d",
			minTerminalWidth,
			minTerminalHeight,
		)
		styled := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#C62828")).
			Padding(1, 2).
			Render(msg)
		if m.width <= 0 || m.height <= 0 {
			return styled
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, styled)
	}

	var content string
	var errorBanner string

	// Show dedicated loading screen when scanning many repositories
	if m.loading && len(m.directories) > loadingScreenThreshold {
		content = m.renderLoadingScreen()
		statusBar := m.renderStatusBar()
		return lipgloss.JoinVertical(lipgloss.Left, content, statusBar)
	}

	if m.err != nil {
		errText := formatErrorForDisplay(m.err)
		trimWidth := m.width
		switch {
		case trimWidth > 2:
			errText = truncateString(errText, trimWidth-2)
		case trimWidth > 0:
			errText = truncateString(errText, trimWidth)
		}
		errorBanner = m.styles.Error.Width(m.width).Render(" " + errText)
	}

	content = m.renderOverview()

	if m.sidePanel != NonePanel {
		popup := m.renderPanelPopup()
		content = lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, popup,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	if m.showHelp {
		help := m.renderHelp()
		content = lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, help,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	if errorBanner != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, errorBanner, content)
	}

	if m.commitPromptActive {
		if prompt := m.renderCommitPrompt(); prompt != "" {
			content = lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, prompt,
				lipgloss.WithWhitespaceChars(" "),
			)
		}
	}

	if m.branchPromptActive {
		if prompt := m.renderBranchPrompt(); prompt != "" {
			content = lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, prompt,
				lipgloss.WithWhitespaceChars(" "),
			)
		}
	}

	if m.worktreePromptActive {
		if prompt := m.renderWorktreePrompt(); prompt != "" {
			content = lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, prompt,
				lipgloss.WithWhitespaceChars(" "),
			)
		}
	}

	if m.stashPromptActive {
		if prompt := m.renderStashPrompt(); prompt != "" {
			content = lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, prompt,
				lipgloss.WithWhitespaceChars(" "),
			)
		}
	}

	if m.activeCredentialPrompt != nil {
		if prompt := m.renderCredentialPrompt(); prompt != "" {
			content = lipgloss.JoinVertical(lipgloss.Left, content, prompt)
		}
	}

	// Status bar is always at the bottom
	statusBar := m.renderStatusBar()

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
func (m *Model) renderOverviewTitleBar() string {
	if m.width <= 0 {
		return ""
	}
	leftTitle := fmt.Sprintf(" Repositories (%d)", len(m.repositories))
	if m.worktreeMode {
		leftTitle = fmt.Sprintf(" Worktree mode (%d)", len(m.worktreeFamilies()))
	}
	rightTitle := fmt.Sprintf("Gitbatch %s ", m.version)
	contentWidth := m.width - 2 // title style adds one space padding on each side
	if contentWidth < 1 {
		contentWidth = 1
	}
	rightWidth := lipgloss.Width(rightTitle)
	if rightWidth > contentWidth {
		rightTitle = truncateString(rightTitle, contentWidth)
		rightWidth = lipgloss.Width(rightTitle)
	}
	minGap := 1
	availableForLeft := contentWidth - rightWidth - minGap
	if availableForLeft < 0 {
		availableForLeft = 0
	}
	leftRendered := truncateString(leftTitle, availableForLeft)
	leftWidth := lipgloss.Width(leftRendered)
	spacing := contentWidth - leftWidth - rightWidth
	if spacing < minGap {
		spacing = minGap
	}
	titleText := leftRendered + strings.Repeat(" ", spacing) + rightTitle
	if lipgloss.Width(titleText) < contentWidth {
		titleText += strings.Repeat(" ", contentWidth-lipgloss.Width(titleText))
	}
	return m.styles.Title.Width(m.width).Render(titleText)
}

func (m *Model) renderOverview() string {
	if len(m.repositories) == 0 {
		if m.loading {
			return m.styles.List.Render("Loading repositories...")
		}
		return m.styles.List.Render("No repositories found")
	}
	if m.worktreeMode {
		return m.renderWorktreeOverview()
	}

	// Calculate visible range based on terminal height
	// Reserve space for: title (1) + top border (1) + bottom border (1) + status bar (1)
	visibleHeight := m.height - 4

	// Compute column widths based on content and available width (cached)
	colWidths := m.getColumnWidths()

	title := m.renderOverviewTitleBar()

	// Compute cumulative row offsets for each repo (accounts for expanded branches)
	repoRowStart := make([]int, len(m.repositories))
	totalRows := 0
	for i, r := range m.repositories {
		repoRowStart[i] = totalRows
		totalRows += m.repoRowCount(r)
	}

	// Determine the scroll window: which visual rows are visible
	topRow := 0
	if totalRows > visibleHeight && m.cursor < len(m.repositories) {
		cursorRow := repoRowStart[m.cursor]
		topRow = cursorRow - visibleHeight/2
		if topRow < 0 {
			topRow = 0
		}
		if topRow+visibleHeight > totalRows {
			topRow = totalRows - visibleHeight
			if topRow < 0 {
				topRow = 0
			}
		}
	}
	bottomRow := topRow + visibleHeight
	if bottomRow > totalRows {
		bottomRow = totalRows
	}

	// Find the first repo that has rows in the visible window
	startIdx := 0
	for i, r := range m.repositories {
		if repoRowStart[i]+m.repoRowCount(r) > topRow {
			startIdx = i
			break
		}
	}

	// Find the last repo that has rows in the visible window
	endIdx := len(m.repositories)
	for i := startIdx; i < len(m.repositories); i++ {
		if repoRowStart[i] >= bottomRow {
			endIdx = i
			break
		}
	}

	var topLabel, bottomLabel string
	if topRow > 0 {
		topLabel = "↑ more above"
	}
	if bottomRow < totalRows {
		bottomLabel = "↓ more below"
	}

	// Top border for table
	topBorder := m.renderTableBorder(colWidths, "top", topLabel)

	// Render repositories with optional expanded branches
	var lines []string
	for i := startIdx; i < endIdx && len(lines) < visibleHeight; i++ {
		r := m.repositories[i]
		rowBase := repoRowStart[i]

		// Primary repo line
		if rowBase >= topRow {
			selected := i == m.cursor
			lines = append(lines, m.renderRepositoryLine(r, selected, colWidths))
		}

		// Expanded branch lines (non-HEAD branches)
		if m.expandBranches && r.State != nil && r.State.Branch != nil {
			style := m.repoUnselectedStyle(r)
			headName := r.State.Branch.Name
			for j, branch := range r.Branches {
				if branch == nil || branch.Name == headName {
					continue
				}
				branchRowPos := rowBase + 1 + m.expandedBranchOffset(r, j)
				if branchRowPos < topRow {
					continue
				}
				if len(lines) >= visibleHeight {
					break
				}
				lines = append(lines, m.renderExpandedBranchLine(r, branch, style, colWidths))
			}
		}
	}

	// Fill remaining rows with empty table rows to stretch to full height
	for len(lines) < visibleHeight {
		border := m.styles.TableBorder.Render("│")
		emptyRepoCol := strings.Repeat(" ", colWidths.repo)
		emptyBranchCol := strings.Repeat(" ", colWidths.branch)
		emptyCommitCol := strings.Repeat(" ", colWidths.commitMsg)
		emptyRow := border + emptyRepoCol + border + emptyBranchCol + border + emptyCommitCol + border
		lines = append(lines, emptyRow)
	}

	// Bottom border for table
	bottomBorder := m.renderTableBorder(colWidths, "bottom", bottomLabel)

	list := strings.Join(lines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, title, topBorder, list, bottomBorder)
}

func (m *Model) renderWorktreeOverview() string {
	rows := m.overviewRows()
	if len(rows) == 0 {
		return m.styles.List.Render("No repositories found")
	}

	visibleHeight := m.height - 4
	colWidths := m.getColumnWidths()
	title := m.renderOverviewTitleBar()

	topRow := 0
	if len(rows) > visibleHeight && m.cursor < len(rows) {
		topRow = m.cursor - visibleHeight/2
		if topRow < 0 {
			topRow = 0
		}
		if topRow+visibleHeight > len(rows) {
			topRow = len(rows) - visibleHeight
			if topRow < 0 {
				topRow = 0
			}
		}
	}
	bottomRow := topRow + visibleHeight
	if bottomRow > len(rows) {
		bottomRow = len(rows)
	}

	var topLabel, bottomLabel string
	if topRow > 0 {
		topLabel = "↑ more above"
	}
	if bottomRow < len(rows) {
		bottomLabel = "↓ more below"
	}

	topBorder := m.renderTableBorder(colWidths, "top", topLabel)
	lines := make([]string, 0, visibleHeight)
	for i := topRow; i < bottomRow && len(lines) < visibleHeight; i++ {
		row := rows[i]
		selected := i == m.cursor && row.selectable()
		switch row.kind {
		case overviewWorktreeRow:
			lines = append(lines, m.renderWorktreeLine(row, selected, colWidths))
		default:
			lines = append(lines, m.renderWorktreeRepositoryLine(row, selected, colWidths))
		}
	}

	for len(lines) < visibleHeight {
		border := m.styles.TableBorder.Render("│")
		emptyRepoCol := strings.Repeat(" ", colWidths.repo)
		emptyBranchCol := strings.Repeat(" ", colWidths.branch)
		emptyCommitCol := strings.Repeat(" ", colWidths.commitMsg)
		lines = append(lines, border+emptyRepoCol+border+emptyBranchCol+border+emptyCommitCol+border)
	}

	bottomBorder := m.renderTableBorder(colWidths, "bottom", bottomLabel)
	return lipgloss.JoinVertical(lipgloss.Left, title, topBorder, strings.Join(lines, "\n"), bottomBorder)
}

// applyUnselectedColumnStyle applies the appropriate lipgloss style to a column string
// when the row is not selected. Selected rows are handled by the highlight block instead.
func (m *Model) applyUnselectedColumnStyle(col string, selected, requiresCredentials, hasLocalChanges, dirty, failed, noUpstream bool) string {
	if selected {
		return col
	}
	if requiresCredentials {
		return m.styles.CredentialsItem.Render(col)
	}
	if hasLocalChanges && !dirty && !failed {
		return m.styles.LocalChangesItem.Render(col)
	}
	if (dirty && !failed || noUpstream) && !requiresCredentials {
		return m.styles.DisabledItem.Render(col)
	}
	return col
}

type repoVisualState struct {
	statusIcon          string
	style               lipgloss.Style
	linkedWorktree      bool
	dirty               bool
	failed              bool
	requiresCredentials bool
	hasLocalChanges     bool
	noUpstream          bool
}

func (m *Model) repoVisualStateFor(r *git.Repository) repoVisualState {
	state := repoVisualState{
		statusIcon: " ",
		style:      m.styles.ListItem,
	}
	if r == nil {
		return state
	}

	status := r.WorkStatus()
	state.linkedWorktree = r.IsLinkedWorktree()
	state.dirty = repoIsDirty(r)
	state.hasLocalChanges = repoHasLocalChanges(r)
	state.failed = status == git.Fail
	state.requiresCredentials = state.failed && r.State != nil && r.State.RequiresCredentials
	state.noUpstream = state.failed && r.State != nil && r.State.NoUpstream

	switch status {
	case git.Pending:
		state.statusIcon = waitingSymbol
		state.style = m.styles.PendingItem
	case git.Queued:
		state.statusIcon = queuedSymbol
		state.style = m.styles.QueuedItem
	case git.Working:
		if len(spinnerFrames) > 0 {
			state.statusIcon = spinnerFrames[m.spinnerIndex%len(spinnerFrames)]
		} else {
			state.statusIcon = "*"
		}
		state.style = m.styles.WorkingItem
	case git.Success:
		state.statusIcon = successSymbol
		state.style = m.styles.SuccessItem
	case git.Fail:
		if state.noUpstream {
			if state.dirty {
				state.statusIcon = localChangesSymbol
			}
			state.style = m.styles.DisabledItem
		} else if state.requiresCredentials {
			state.style = m.styles.CredentialsItem
		} else {
			state.statusIcon = failSymbol
			state.style = m.styles.FailedItem
		}
	}
	if state.hasLocalChanges && !state.dirty && !state.failed && status.Ready {
		state.statusIcon = localChangesSymbol
		state.style = m.styles.LocalChangesItem
	}
	if state.dirty && !state.failed && status.Ready {
		state.statusIcon = dirtySymbol
		state.style = m.styles.DisabledItem
	}
	return state
}

func (m *Model) selectedHighlightForVisual(visual repoVisualState) lipgloss.Style {
	switch {
	case visual.noUpstream:
		return m.styles.DisabledSelectedItem
	case visual.requiresCredentials:
		return m.styles.CredentialsSelectedItem
	case visual.failed:
		return m.styles.FailedSelectedItem
	case visual.dirty:
		return m.styles.DisabledSelectedItem
	case visual.hasLocalChanges:
		return m.styles.LocalChangesSelectedItem
	case visual.linkedWorktree:
		return m.styles.WorktreeSelectedItem
	default:
		return m.styles.SelectedItem
	}
}

// renderRepositoryLine renders a single repository line as a table row
// Table format: │cursor status repo-name    │ branch-name │ commit tags/message │
// Example:      │→ ●   example-repo         │  main       │ [v1.0.0] add feature │
func (m *Model) renderRepositoryLine(r *git.Repository, selected bool, colWidths columnWidths) string {
	visual := m.repoVisualStateFor(r)

	cursor := " "
	if selected {
		cursor = "→"
	}

	repoNameWidth := colWidths.repo - repoColPrefixWidth
	if repoNameWidth < 0 {
		repoNameWidth = 0
	}
	repoName := truncateString(repoDisplayName(r), repoNameWidth)
	repoColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%s %s %-*s", cursor, visual.statusIcon, repoNameWidth, repoName),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	branchContentWidth := colWidths.branch - 1
	if branchContentWidth < 0 {
		branchContentWidth = 0
	}
	branchContent := truncateString(branchContent(r), branchContentWidth)
	branchColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%-*s", colWidths.branch, " "+branchContent),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	commitContentWidth := colWidths.commitMsg - 1
	if commitContentWidth < 0 {
		commitContentWidth = 0
	}

	fullCommitContent := m.commitContentForRepo(r)
	offset := m.getCommitScrollOffset(r)
	maxOffset := maxCommitOffset(fullCommitContent, commitContentWidth)
	if offset > maxOffset {
		offset = maxOffset
		m.setCommitScrollOffset(r, offset)
	} else if maxOffset == 0 && offset != 0 {
		offset = 0
		m.setCommitScrollOffset(r, 0)
	}
	commitContent := visibleCommitContent(fullCommitContent, offset, commitContentWidth)
	commitColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%-*s", colWidths.commitMsg, " "+commitContent),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	var styledRepoCol, styledBranchCol, styledCommitCol string
	if selected {
		highlight := m.selectedHighlightForVisual(visual)
		styledRepoCol = highlight.Render(repoColumn)
		styledBranchCol = highlight.Render(branchColumn)
		styledCommitCol = highlight.Render(commitColumn)
	} else {
		styledRepoCol = visual.style.Render(repoColumn)
		styledBranchCol = visual.style.Render(branchColumn)
		styledCommitCol = visual.style.Render(commitColumn)
	}

	border := m.styles.TableBorder.Render("│")
	return border + styledRepoCol + border + styledBranchCol + border + styledCommitCol + border
}

func (m *Model) renderWorktreeRepositoryLine(row overviewRow, selected bool, colWidths columnWidths) string {
	repo := row.repository()
	visual := m.repoVisualStateFor(repo)

	cursor := " "
	if selected {
		cursor = "→"
	}

	repoNameWidth := colWidths.repo - repoColPrefixWidth
	if repoNameWidth < 0 {
		repoNameWidth = 0
	}
	repoBody := renderRepoColumnBody(repoDisplayName(repo), repoNameWidth, "", 0)
	repoColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%s %s %s", cursor, visual.statusIcon, repoBody),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	branchWidth := colWidths.branch - 1
	if branchWidth < 0 {
		branchWidth = 0
	}
	branchColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%-*s", colWidths.branch, " "+truncateString(row.worktreeLabel(), branchWidth)),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	commitWidth := colWidths.commitMsg - 1
	if commitWidth < 0 {
		commitWidth = 0
	}
	fullCommitContent := m.commitContentForRepo(repo)
	offset := m.getCommitScrollOffset(repo)
	maxOffset := maxCommitOffset(fullCommitContent, commitWidth)
	if offset > maxOffset {
		offset = maxOffset
		m.setCommitScrollOffset(repo, offset)
	} else if maxOffset == 0 && offset != 0 {
		offset = 0
		m.setCommitScrollOffset(repo, 0)
	}
	commitContent := visibleCommitContent(fullCommitContent, offset, commitWidth)
	commitColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%-*s", colWidths.commitMsg, " "+commitContent),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	var styledRepoCol, styledBranchCol, styledCommitCol string
	if selected {
		highlight := m.selectedHighlightForVisual(visual)
		styledRepoCol = highlight.Render(repoColumn)
		styledBranchCol = highlight.Render(branchColumn)
		styledCommitCol = highlight.Render(commitColumn)
	} else {
		styledRepoCol = visual.style.Render(repoColumn)
		styledBranchCol = visual.style.Render(branchColumn)
		styledCommitCol = visual.style.Render(commitColumn)
	}

	border := m.styles.TableBorder.Render("│")
	return border + styledRepoCol + border + styledBranchCol + border + styledCommitCol + border
}

func (m *Model) renderWorktreeLine(row overviewRow, selected bool, colWidths columnWidths) string {
	repo := row.repository()
	visual := m.repoVisualStateFor(repo)

	cursor := " "
	if selected {
		cursor = "→"
	}

	repoNameWidth := colWidths.repo - repoColPrefixWidth
	if repoNameWidth < 0 {
		repoNameWidth = 0
	}
	repoName := ""
	diffContent := ""
	diffWidth := 0
	if row.worktree != nil && row.worktree.IsPrimary {
		repoName = repoDisplayName(row.actionRepository())
		diffContent, diffWidth = m.worktreeDiffContent(row.actionRepository(), selected)
	}
	repoColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%s %s %s", cursor, visual.statusIcon, renderRepoColumnBody(repoName, repoNameWidth, diffContent, diffWidth)),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	worktreeWidth := colWidths.branch - 1
	if worktreeWidth < 0 {
		worktreeWidth = 0
	}
	worktreeLabel := truncateString(m.worktreeBranchContent(row), worktreeWidth)
	branchColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%-*s", colWidths.branch, " "+worktreeLabel),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	commitWidth := colWidths.commitMsg - 1
	if commitWidth < 0 {
		commitWidth = 0
	}
	commitContent := ""
	if repo != nil {
		fullCommitContent := m.commitContentForRepo(repo)
		offset := m.getCommitScrollOffset(repo)
		maxOffset := maxCommitOffset(fullCommitContent, commitWidth)
		if offset > maxOffset {
			offset = maxOffset
			m.setCommitScrollOffset(repo, offset)
		}
		commitContent = visibleCommitContent(fullCommitContent, offset, commitWidth)
	}
	commitColumn := m.applyUnselectedColumnStyle(
		fmt.Sprintf("%-*s", colWidths.commitMsg, " "+commitContent),
		selected, visual.requiresCredentials, visual.hasLocalChanges, visual.dirty, visual.failed, visual.noUpstream,
	)

	var styledRepoCol, styledBranchCol, styledCommitCol string
	if selected {
		highlight := m.selectedHighlightForVisual(visual)
		styledRepoCol = highlight.Render(repoColumn)
		styledBranchCol = highlight.Render(branchColumn)
		styledCommitCol = highlight.Render(commitColumn)
	} else {
		styledRepoCol = visual.style.Render(repoColumn)
		styledBranchCol = visual.style.Render(branchColumn)
		styledCommitCol = visual.style.Render(commitColumn)
	}

	border := m.styles.TableBorder.Render("│")
	return border + styledRepoCol + border + styledBranchCol + border + styledCommitCol + border
}

// expandedBranchOffset returns the visual row offset (from 0) for a non-HEAD branch
// at index branchIdx, skipping the HEAD branch in the count.
func (m *Model) expandedBranchOffset(r *git.Repository, branchIdx int) int {
	if r == nil || r.State == nil || r.State.Branch == nil {
		return branchIdx
	}
	headName := r.State.Branch.Name
	offset := 0
	for i := 0; i < branchIdx; i++ {
		if r.Branches[i] != nil && r.Branches[i].Name != headName {
			offset++
		}
	}
	return offset
}

// repoRowCount returns how many visual rows a repository occupies.
func (m *Model) repoRowCount(r *git.Repository) int {
	if !m.expandBranches || r == nil || len(r.Branches) <= 1 {
		return 1
	}
	return len(r.Branches)
}

// repoUnselectedStyle determines the lipgloss style for a repo's rows (unselected).
func (m *Model) repoUnselectedStyle(r *git.Repository) lipgloss.Style {
	status := r.WorkStatus()
	dirty := repoIsDirty(r)
	hasLocalChanges := repoHasLocalChanges(r)
	failed := status == git.Fail
	requiresCredentials := failed && r.State != nil && r.State.RequiresCredentials
	noUpstream := failed && r.State != nil && r.State.NoUpstream

	style := m.styles.ListItem
	switch status {
	case git.Pending:
		style = m.styles.PendingItem
	case git.Queued:
		style = m.styles.QueuedItem
	case git.Working:
		style = m.styles.WorkingItem
	case git.Success:
		style = m.styles.SuccessItem
	case git.Fail:
		if noUpstream {
			style = m.styles.DisabledItem
		} else if requiresCredentials {
			style = m.styles.CredentialsItem
		} else {
			style = m.styles.FailedItem
		}
	}
	if hasLocalChanges && !dirty && !failed && status.Ready {
		style = m.styles.LocalChangesItem
	}
	if dirty && !failed && status.Ready {
		style = m.styles.DisabledItem
	}
	return style
}

// renderExpandedBranchLine renders a single expanded branch row for a non-HEAD branch.
// It uses the same color as the repo's primary row but without status icons or repo name.
func (m *Model) renderExpandedBranchLine(r *git.Repository, branch *git.Branch, style lipgloss.Style, colWidths columnWidths) string {
	// Empty repo column (spaces matching cursor + status + repo name width)
	repoColumn := style.Render(strings.Repeat(" ", colWidths.repo))

	// Branch column
	branchContentWidth := colWidths.branch - 1
	if branchContentWidth < 0 {
		branchContentWidth = 0
	}
	branchStr := branch.Name + syncSuffix(branch)
	branchStr = truncateString(branchStr, branchContentWidth)
	branchColumn := style.Render(fmt.Sprintf("%-*s", colWidths.branch, " "+branchStr))

	// Commit column — look up last commit for this branch
	commitContentWidth := colWidths.commitMsg - 1
	if commitContentWidth < 0 {
		commitContentWidth = 0
	}
	commitStr := m.branchCommitContent(r, branch)
	commitStr = truncateString(commitStr, commitContentWidth)
	commitColumn := style.Render(fmt.Sprintf("%-*s", colWidths.commitMsg, " "+commitStr))

	border := m.styles.TableBorder.Render("│")
	return border + repoColumn + border + branchColumn + border + commitColumn + border
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

func maxCommitOffset(content string, width int) int {
	if width <= 0 {
		return 0
	}
	runeCount := len([]rune(content))
	if runeCount <= width {
		return 0
	}
	return runeCount - width
}

func visibleCommitContent(content string, offset, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(content)
	total := len(runes)
	if total == 0 {
		return ""
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + width
	if end > total {
		end = total
	}
	if offset >= end {
		return ""
	}
	visible := append([]rune{}, runes[offset:end]...)
	if len(visible) == 0 {
		return ""
	}
	if offset > 0 {
		visible[0] = '…'
	}
	if end < total {
		visible[len(visible)-1] = '…'
	}
	return string(visible)
}

func firstLine(message string) string {
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		message = message[:idx]
	}
	return strings.TrimSpace(message)
}

func singleLineMessage(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\v", " ")
	message = strings.ReplaceAll(message, "\f", " ")
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}
	return strings.Join(strings.Fields(message), " ")
}

func formatErrorForDisplay(err error) string {
	if err == nil {
		return ""
	}
	return singleLineMessage("Error: " + err.Error())
}

// truncateString truncates a string to the specified length, adding "..." if needed
func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		runes := []rune(s)
		if len(runes) > maxLen {
			runes = runes[:maxLen]
		}
		return string(runes)
	}
	b := strings.Builder{}
	current := 0
	for _, r := range s {
		rWidth := lipgloss.Width(string(r))
		if current+rWidth > maxLen-3 {
			break
		}
		b.WriteRune(r)
		current += rWidth
	}
	b.WriteString("...")
	return b.String()
}

// renderPanelPopup renders a panel (branches, remotes, status, stash) as a centered popup.
func (m *Model) renderPanelPopup() string {
	if len(m.repositories) == 0 {
		return ""
	}

	popupWidth, maxContentLines := m.popupDimensions()
	contentWidth := popupWidth - panelHorizontalFrame
	if contentWidth < 1 {
		contentWidth = 1
	}

	r := m.currentRepository()
	if r == nil {
		return ""
	}

	// Build header with repo info
	var header []string
	tagged := m.taggedRepositories()
	if len(tagged) > 1 {
		header = append(header, fmt.Sprintf("%d tagged repositories", len(tagged)))
	} else {
		repoName := r.Name
		if r.State != nil && r.State.Branch != nil {
			repoName += "  " + m.styles.BranchInfo.Render(r.State.Branch.Name)
			if r.State.Branch.Upstream != nil {
				repoName += " → " + m.styles.BranchInfo.Render(r.State.Branch.Upstream.Name)
			}
		}
		header = append(header, repoName)
	}

	// Panel title
	var panelTitle string
	switch m.sidePanel {
	case BranchPanel:
		if len(tagged) > 1 {
			panelTitle = "Common Branches"
		} else {
			panelTitle = "Branches"
		}
	case RemotePanel:
		if len(tagged) > 1 {
			panelTitle = "Common Remote Branches"
		} else {
			panelTitle = "Remotes"
		}
	case StatusPanel:
		panelTitle = "Status"
	case StashActionPanel:
		if m.stashAction == stashActionPop {
			panelTitle = "Pop Stash"
		} else {
			panelTitle = "Drop Stash"
		}
		if len(tagged) > 1 {
			panelTitle = "Common " + panelTitle
		}
	default:
		return ""
	}

	// Reserve lines for header (header lines + blank separator + title)
	headerLines := len(header) + 2 // header + blank + title already in panelContent
	maxLines := maxContentLines - headerLines
	if maxLines < 1 {
		maxLines = 1
	}

	// Render panel content
	var panelContent string
	switch m.sidePanel {
	case BranchPanel:
		panelContent = m.renderBranches(contentWidth, maxLines)
	case RemotePanel:
		panelContent = m.renderRemotes(contentWidth, maxLines)
	case StatusPanel:
		panelContent = m.renderStatus(r, contentWidth, maxLines)
	case StashActionPanel:
		panelContent = m.renderStashActionPanel(contentWidth, maxLines)
	}

	// Assemble popup content
	parts := []string{
		m.styles.PanelTitle.Render(panelTitle),
		strings.Join(header, "\n"),
		"",
		panelContent,
	}
	panel := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Style the panel
	panelStyle := m.styles.Panel.Width(popupWidth)
	if (m.sidePanel == BranchPanel || m.sidePanel == RemotePanel) && len(tagged) > 1 {
		var panelHasItems bool
		switch m.sidePanel {
		case BranchPanel:
			panelHasItems = len(m.branchPanelItems()) > 0
		case RemotePanel:
			panelHasItems = len(m.remotePanelItems()) > 0
		}
		if panelHasItems {
			panelStyle = panelStyle.BorderForeground(commonPanelBorderColor)
		}
	}

	return panelStyle.Render(panel)
}

// renderBranches renders branch list
func (m *Model) renderBranches(contentWidth, maxLines int) string {
	items := m.branchPanelItems()
	if len(items) == 0 {
		if m.hasMultipleTagged() {
			return padToWidth("No common branches", contentWidth)
		}
		return padToWidth("No branches", contentWidth)
	}

	if contentWidth <= 0 || maxLines <= 0 {
		return ""
	}

	count := len(items)
	viewport := m.branchViewportSize(count)
	if viewport <= 0 {
		viewport = count
	}
	if viewport > count {
		viewport = count
	}

	m.ensureBranchCursorVisible(count, viewport)

	start := m.branchOffset
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

	lines := make([]string, 0, maxLines)
	instructions := fmt.Sprintf("%s checkout  %s new  %s delete",
		m.styles.KeyBinding.Render("[space/c]"),
		m.styles.KeyBinding.Render("[n]"),
		m.styles.KeyBinding.Render("[d]"),
	)
	lines = append(lines, padToWidth(instructions, contentWidth))

	remaining := maxLines - 1
	if remaining <= 0 {
		return strings.Join(lines, "\n")
	}

	lines = append(lines, padToWidth("", contentWidth))
	remaining--
	if remaining <= 0 {
		return strings.Join(lines, "\n")
	}

	if start > 0 {
		hint := m.styles.Help.Render("  ↑ more above")
		lines = append(lines, padToWidth(hint, contentWidth))
		remaining--
	}

	selectedStyle := m.styles.SelectedItem
	if m.hasMultipleTagged() && len(items) > 0 {
		selectedStyle = m.styles.CommonSelectedItem
	}
	for i := start; i < end && remaining > 0; i++ {
		item := items[i]
		prefix := "  "
		if item.IsCurrent {
			prefix = "→ "
		}
		line := prefix + item.Name
		if i == m.branchCursor {
			line = selectedStyle.Render(padToWidth(line, contentWidth))
		} else {
			line = padToWidth(line, contentWidth)
		}
		lines = append(lines, line)
		remaining--
	}

	if remaining > 0 && end < count {
		hint := m.styles.Help.Render("  ↓ more below")
		lines = append(lines, padToWidth(hint, contentWidth))
	}

	lines = clampLines(lines, maxLines)
	return strings.Join(lines, "\n")
}

// renderRemotes renders remote list
func (m *Model) renderRemotes(contentWidth, maxLines int) string {
	items := m.remotePanelItems()
	if len(items) == 0 {
		if m.hasMultipleTagged() {
			return padToWidth("No common remote branches", contentWidth)
		}
		return padToWidth("No remote branches", contentWidth)
	}

	if contentWidth <= 0 || maxLines <= 0 {
		return ""
	}

	count := len(items)
	viewport := m.remoteViewportSize(count)
	if viewport <= 0 {
		viewport = count
	}
	if viewport > count {
		viewport = count
	}

	m.ensureRemoteCursorVisible(count, viewport)

	start := m.remoteOffset
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

	lines := make([]string, 0, maxLines)
	instructions := fmt.Sprintf("%s checkout  %s delete",
		m.styles.KeyBinding.Render("[space/c]"),
		m.styles.KeyBinding.Render("[d]"),
	)
	lines = append(lines, padToWidth(instructions, contentWidth))

	remaining := maxLines - 1
	if remaining <= 0 {
		return strings.Join(lines, "\n")
	}

	lines = append(lines, padToWidth("", contentWidth))
	remaining--
	if remaining <= 0 {
		return strings.Join(lines, "\n")
	}

	if start > 0 {
		hint := m.styles.Help.Render("  ↑ more above")
		lines = append(lines, padToWidth(hint, contentWidth))
		remaining--
	}

	selectedStyle := m.styles.SelectedItem
	if m.hasMultipleTagged() && len(items) > 0 {
		selectedStyle = m.styles.CommonSelectedItem
	}
	for i := start; i < end && remaining > 0; i++ {
		item := items[i]
		line := fmt.Sprintf("%s %s", item.RemoteName, item.BranchName)
		if i == m.remoteBranchCursor {
			line = selectedStyle.Render(padToWidth(line, contentWidth))
		} else {
			line = padToWidth(line, contentWidth)
		}
		lines = append(lines, line)
		remaining--
	}

	if remaining > 0 && end < count {
		hint := m.styles.Help.Render("  ↓ more below")
		lines = append(lines, padToWidth(hint, contentWidth))
	}

	lines = clampLines(lines, maxLines)
	return strings.Join(lines, "\n")
}

// renderStatus renders git status
func (m *Model) renderStatus(r *git.Repository, contentWidth, maxLines int) string {
	if contentWidth <= 0 || maxLines <= 0 {
		return ""
	}

	lines := make([]string, 0, maxLines)

	addLine := func(s string) bool {
		if len(lines) >= maxLines {
			return false
		}
		lines = append(lines, padToWidth(s, contentWidth))
		return len(lines) < maxLines
	}

	addSection := func() bool {
		return addLine("")
	}

	// Branch & tracking
	addLine("On branch " + m.styles.BranchInfo.Render(r.State.Branch.Name))

	pushables, _ := strconv.Atoi(r.State.Branch.Pushables)
	pullables, _ := strconv.Atoi(r.State.Branch.Pullables)

	switch {
	case r.State.Branch.Upstream == nil:
		addLine("Not tracking a remote branch")
	case pushables == 0 && pullables == 0:
		addLine("Up to date with " + m.styles.BranchInfo.Render(r.State.Branch.Upstream.Name))
	default:
		if pushables > 0 && pullables > 0 {
			addLine(fmt.Sprintf("Diverged from %s (ahead %d, behind %d)", r.State.Branch.Upstream.Name, pushables, pullables))
		} else if pushables > 0 {
			addLine(fmt.Sprintf("Ahead of %s by %d commit(s)", r.State.Branch.Upstream.Name, pushables))
		} else {
			addLine(fmt.Sprintf("Behind %s by %d commit(s)", r.State.Branch.Upstream.Name, pullables))
		}
	}

	if r.State.Branch.HasLocalChanges {
		addLine("Working tree has uncommitted changes")
	} else if !r.State.Branch.Clean {
		addLine("Working tree is dirty (conflicts with incoming)")
	}

	// Fetch additional stats via git commands
	stats := m.fetchRepoStats(r)

	// Last commit
	if stats.lastCommitInfo != "" {
		addSection()
		addLine("Last commit")
		addLine("  " + stats.lastCommitInfo)
	}

	// Counts section
	addSection()
	addLine(fmt.Sprintf("Branches       %d local, %d remote", stats.localBranches, stats.remoteBranches))
	if stats.tags > 0 {
		addLine(fmt.Sprintf("Tags           %d", stats.tags))
	}
	if stats.stashes > 0 {
		addLine(fmt.Sprintf("Stashes        %d", stats.stashes))
	}
	addLine(fmt.Sprintf("Contributors   %d", stats.contributors))
	addLine(fmt.Sprintf("Commits        %d", stats.commits))

	// Size
	if stats.repoSize != "" {
		addSection()
		addLine(fmt.Sprintf("Repo size      %s", stats.repoSize))
	}

	// Path
	addSection()
	addLine(r.AbsPath)

	return strings.Join(clampLines(lines, maxLines), "\n")
}

type repoStats struct {
	lastCommitInfo string
	localBranches  int
	remoteBranches int
	tags           int
	stashes        int
	contributors   int
	commits        int
	repoSize       string
}

func (m *Model) fetchRepoStats(r *git.Repository) repoStats {
	var stats repoStats

	// Local & remote branches from already-loaded data
	stats.localBranches = len(r.Branches)
	for _, remote := range r.Remotes {
		if remote != nil {
			stats.remoteBranches += len(remote.Branches)
		}
	}

	// Stashes from loaded data
	stats.stashes = len(r.Stasheds)

	// Last commit (git log -1)
	if out, err := statusGitCommand(r.AbsPath, "log", "-1", "--format=%ar by %an: %s"); err == nil && out != "" {
		stats.lastCommitInfo = out
	}

	// Tag count
	if out, err := statusGitCommand(r.AbsPath, "tag", "-l"); err == nil {
		count := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
		stats.tags = count
	}

	// Contributor count (unique authors)
	if out, err := statusGitCommand(r.AbsPath, "shortlog", "-sn", "--all"); err == nil {
		count := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
		stats.contributors = count
	}

	// Commit count
	if out, err := statusGitCommand(r.AbsPath, "rev-list", "--count", "HEAD"); err == nil {
		out = strings.TrimSpace(out)
		if n, err := strconv.Atoi(out); err == nil {
			stats.commits = n
		}
	}

	// Repo size (du on .git directory)
	if out, err := repoSizeCommand(r.AbsPath); err == nil {
		stats.repoSize = out
	}

	return stats
}

func statusGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func repoSizeCommand(dir string) (string, error) {
	gitDir := dir + "/.git"
	cmd := exec.Command("du", "-sh", gitDir)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) > 0 {
		return parts[0], nil
	}
	return "", fmt.Errorf("no output")
}

// renderStashActionPanel renders the stash selector for pop/drop operations
func (m *Model) renderStashActionPanel(contentWidth, maxLines int) string {
	if contentWidth <= 0 || maxLines <= 0 {
		return ""
	}

	items := m.stashActionPanelItems()
	if len(items) == 0 {
		return padToWidth("No stashes", contentWidth)
	}

	viewport := maxLines
	if viewport > len(items) {
		viewport = len(items)
	}

	lines := make([]string, 0, viewport)
	for i := m.stashOffset; i < len(items) && len(lines) < viewport; i++ {
		item := items[i]
		label := item.Description
		if label == "" {
			label = fmt.Sprintf("stash@{%d} on %s", item.StashID, item.BranchName)
		}
		line := padToWidth("  "+label, contentWidth)
		if i == m.stashCursor {
			line = m.styles.SelectedItem.Render(padToWidth("> "+label, contentWidth))
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m *Model) renderStashPrompt() string {
	if !m.stashPromptActive {
		return ""
	}
	panelWidth := 60
	if m.width > 0 && m.width-4 < panelWidth {
		panelWidth = m.width - 4
	}
	if panelWidth < 30 {
		panelWidth = 30
	}
	contentWidth := panelWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Title
	repoCount := len(m.stashPromptRepos)
	var title string
	if repoCount == 1 && m.stashPromptRepos[0] != nil {
		title = fmt.Sprintf("Stash in %s", truncateString(m.stashPromptRepos[0].Name, contentWidth-10))
	} else {
		title = fmt.Sprintf("Stash in %d repos", repoCount)
	}

	// Message field
	msgDisplay := m.stashMessageBuffer
	if len(msgDisplay) > contentWidth-2 {
		msgDisplay = msgDisplay[len(msgDisplay)-contentWidth+2:]
	}

	msgLine := fmt.Sprintf("> Message: %s_", msgDisplay)
	hint := "(optional, Enter to stash, Esc to cancel)"

	parts := []string{
		m.styles.PanelTitle.Render(title),
		"",
		msgLine,
		"",
		m.styles.Help.Render(hint),
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return m.styles.Panel.Width(panelWidth).Render(body)
}

// hasCommitTargets returns true if pressing 'c' would find repos with local changes.
func (m *Model) hasCommitTargets() bool {
	repos := m.taggedRepositories()
	if len(repos) == 0 {
		return repoHasLocalChanges(m.currentRepository())
	}
	for _, repo := range repos {
		if repoHasLocalChanges(repo) {
			return true
		}
	}
	return false
}

func (m *Model) hasBranchTargets() bool {
	if len(filterRepositories(m.taggedRepositories())) > 0 {
		return true
	}
	return m.currentRepository() != nil
}

// hasStashTargets returns true if pressing 'O' or 'D' would find repos with stashes.
func (m *Model) hasStashTargets() bool {
	repos := m.taggedRepositories()
	if len(repos) == 0 {
		repo := m.currentRepository()
		return repo != nil && len(repo.Stasheds) > 0
	}
	for _, repo := range repos {
		if repo != nil && len(repo.Stasheds) > 0 {
			return true
		}
	}
	return false
}

func (m *Model) worktreeStatusHints() []string {
	if !m.worktreeMode {
		return nil
	}

	hints := []string{"W branches", "n worktree", "X prune"}
	if row, ok := m.currentOverviewRow(); ok && row.kind == overviewWorktreeRow && row.worktree != nil && !row.worktree.IsPrimary {
		if row.worktree.IsLocked {
			hints = append(hints, "L unlock")
		} else {
			hints = append(hints, "d delete", "L lock")
		}
	}
	return hints
}

// renderStatusBar renders the bottom status bar
func (m *Model) renderStatusBar() string {
	modeSymbol := pullSymbol
	statusBarStyle := m.styles.StatusBarPull
	totalWidth := m.width
	switch m.mode.ID {
	case MergeMode:
		modeSymbol = mergeSymbol
		statusBarStyle = m.styles.StatusBarMerge
	case RebaseMode:
		modeSymbol = rebaseSymbol
		statusBarStyle = m.styles.StatusBarRebase
	case PushMode:
		modeSymbol = pushSymbol
		statusBarStyle = m.styles.StatusBarPush
	}

	left := fmt.Sprintf(" %s %s", modeSymbol, m.mode.DisplayString)

	queuedCount := 0
	for _, r := range m.repositories {
		if r.WorkStatus() == git.Queued {
			queuedCount++
		}
	}

	focusRepo := m.currentRepository()
	linkedWorktree := focusRepo != nil && focusRepo.IsLinkedWorktree()
	dirty := repoIsDirty(focusRepo)
	hasLocalChanges := repoHasLocalChanges(focusRepo) && !dirty
	failed := focusRepo != nil && focusRepo.WorkStatus() == git.Fail
	requiresCredentials := failed && focusRepo.State != nil && focusRepo.State.RequiresCredentials
	noUpstream := failed && focusRepo.State != nil && focusRepo.State.NoUpstream

	center := ""

	right := "TAB: lazygit | ? for help"

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	worktreeHints := m.worktreeStatusHints()
	branchHints := []string(nil)
	if !m.worktreeMode && m.hasBranchTargets() {
		branchHints = append(branchHints, "n branch")
	}

	if linkedWorktree {
		worktreeName := ""
		if row, ok := m.currentOverviewRow(); ok {
			switch {
			case row.worktree != nil:
				worktreeName = row.worktreeLabel()
			case row.repo != nil:
				if current := row.repo.CurrentWorktree(); current != nil {
					worktreeName = trimWorktreeRepositoryPrefix(current.DisplayName(), row.repo)
				}
			}
		}
		if worktreeName == "" {
			worktreeName = "unknown"
		}
		statusBarStyle = m.styles.StatusBarWorktree
		left = " "
		parts := append([]string{"worktree: " + worktreeName}, worktreeHints...)
		center = strings.Join(parts, " | ")
	} else if center == "" {
		tagHint := "space: tag"
		if focusRepo != nil && focusRepo.WorkStatus() == git.Queued {
			tagHint = "space: untag"
		}
		if queuedCount > 0 {
			tagHint += " | enter: start batch"
			parts := []string{fmt.Sprintf("tagged: %d", queuedCount)}
			parts = append(parts, branchHints...)
			if m.hasCommitTargets() {
				parts = append(parts, "c commit", "S stash")
			}
			if m.hasStashTargets() {
				parts = append(parts, "O pop", "D drop")
			}
			parts = append(parts, worktreeHints...)
			parts = append(parts, tagHint)
			center = strings.Join(parts, " | ")
		} else if m.activeForcePrompt == nil && m.activeCredentialPrompt == nil {
			parts := []string{"f fetch", "p pull", "P push"}
			parts = append(parts, branchHints...)
			if m.hasCommitTargets() {
				parts = append(parts, "c commit", "S stash")
			}
			if m.hasStashTargets() {
				parts = append(parts, "O pop", "D drop")
			}
			parts = append(parts, worktreeHints...)
			parts = append(parts, tagHint)
			center = strings.Join(parts, " | ")
		}
	}

	if linkedWorktree {
		// Keep linked worktrees in their dedicated neutral state even when local
		// file changes are present; remote actions are intentionally disabled.
	} else if failed {
		hasMessage := focusRepo != nil && focusRepo.State != nil && focusRepo.State.Message != ""
		message := "Operation failed"
		if hasMessage {
			message = truncateString(singleLineMessage(focusRepo.State.Message), totalWidth)
		}
		if noUpstream {
			statusBarStyle = m.styles.StatusBarDisabled
			left = " no upstream"
			right = "TAB: lazygit"
			rightWidth = lipgloss.Width(right)
			maxCenter := totalWidth - lipgloss.Width(left) - rightWidth - 2
			if maxCenter < 0 {
				maxCenter = 0
			}
			center = truncateString(message, maxCenter)
		} else if requiresCredentials {
			statusBarStyle = m.styles.StatusBarCredentials
			left = " credentials required"
			right = "enter: provide | TAB: lazygit"
			if hasMessage {
				right = "enter: provide | c: clear | TAB: lazygit"
			}
			rightWidth = lipgloss.Width(right)
			maxCenter := totalWidth - lipgloss.Width(left) - rightWidth - 2
			if maxCenter < 0 {
				maxCenter = 0
			}
			center = truncateString(message, maxCenter)
		} else {
			statusBarStyle = m.styles.StatusBarError
			left = " repo failed"
			if hasMessage {
				right = "c: clear | TAB: lazygit"
			} else {
				right = "TAB: lazygit | ? for help"
			}
			rightWidth = lipgloss.Width(right)
			maxCenter := totalWidth - lipgloss.Width(left) - rightWidth - 2
			if maxCenter < 0 {
				maxCenter = 0
			}
			center = truncateString(message, maxCenter)
		}
	} else if dirty {
		statusBarStyle = m.styles.StatusBarDisabled
		left = " repo disabled"
		parts := []string{"working tree has conflicting changes"}
		parts = append(parts, branchHints...)
		if m.hasStashTargets() {
			parts = append(parts, "O: pop stash", "D: drop stash")
		}
		parts = append(parts, worktreeHints...)
		center = strings.Join(parts, " | ")
		right = "TAB: lazygit"
	} else if hasLocalChanges {
		statusBarStyle = m.styles.StatusBarLocalChanges
		left = " ~ local changes"
		parts := []string{"c: commit", "S: stash"}
		parts = append(parts, branchHints...)
		if m.hasStashTargets() {
			parts = append(parts, "O: pop stash", "D: drop stash")
		}
		parts = append(parts, worktreeHints...)
		center = strings.Join(parts, " | ")
	} else if m.err != nil {
		statusBarStyle = m.styles.StatusBarPush
		maxCenter := totalWidth - leftWidth - rightWidth - 2
		if maxCenter < 0 {
			maxCenter = 0
		}
		center = truncateString(formatErrorForDisplay(m.err), maxCenter)
	}
	if m.activeForcePrompt != nil && m.activeForcePrompt.repo != nil {
		statusBarStyle = m.styles.StatusBarPush
		repoName := truncateString(m.activeForcePrompt.repo.Name, 20)
		left = fmt.Sprintf(" %s %s push failed", pushSymbol, repoName)
		center = "Retry push with --force?"
		right = "return: confirm | esc: cancel"
	}

	if m.sidePanel != NonePanel && m.activeCredentialPrompt == nil && m.activeForcePrompt == nil {
		if right == "" {
			right = "esc: back"
		} else if !strings.Contains(strings.ToLower(right), "esc: back") {
			right = right + " | esc: back"
		}
	}

	// Calculate spacing - ensure we don't overflow the width
	leftWidth = lipgloss.Width(left)
	rightWidth = lipgloss.Width(right)
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
Navigation:  up/k up   g/Home top        down/j down  G/End bottom
             PgUp/Ctrl+B page up         PgDn/Ctrl+F  page down
             Ctrl+U half page up         Ctrl+D half page down
             left/h scroll left          right/l scroll right

Actions:     Space   tag/untag repo      Enter   process tagged
             a       tag all             A       untag all
             m       cycle mode          Tab     open lazygit

Views:       b  branches           s  status       r  remotes
             B  expand branches    W  worktrees    R  refresh
             ESC back

Sorting:     t  toggle name/time

Git:         f  fetch repo   p  pull repo   P  push repo
             n  new branch / worktree       d  delete worktree
             L  lock/unlock worktree        X  prune stale worktrees
             c  commit / clear error        S  stash
             O  pop stash    D  drop stash

Other:       ?  help         q/Ctrl+C  quit
`

	title := m.styles.PanelTitle.Render("Help")
	body := lipgloss.JoinVertical(lipgloss.Left, title, m.styles.Help.Render(help))

	panelWidth := 68
	if m.width > 0 && panelWidth > m.width-4 {
		panelWidth = m.width - 4
	}

	return m.styles.Panel.Width(panelWidth).Render(body)
}

func (m *Model) renderCommitPrompt() string {
	if !m.commitPromptActive {
		return ""
	}
	panelWidth := 60
	if m.width > 0 && m.width-4 < panelWidth {
		panelWidth = m.width - 4
	}
	if panelWidth < 30 {
		panelWidth = 30
	}
	contentWidth := panelWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Title
	repoCount := len(m.commitPromptRepos)
	var title string
	if repoCount == 1 && m.commitPromptRepos[0] != nil {
		title = fmt.Sprintf("Commit in %s", truncateString(m.commitPromptRepos[0].Name, contentWidth-10))
	} else {
		title = fmt.Sprintf("Commit in %d repos", repoCount)
	}

	// Commit message field
	msgIndicator := " "
	descIndicator := " "
	if m.commitPromptField == commitFieldMessage {
		msgIndicator = ">"
	} else {
		descIndicator = ">"
	}

	msgDisplay := m.commitMessageBuffer
	if len(msgDisplay) > contentWidth-2 {
		msgDisplay = msgDisplay[len(msgDisplay)-contentWidth+2:]
	}

	// Description field: show up to 5 visible lines
	descLines := splitDescLines(m.commitDescBuffer, contentWidth-2)
	maxVisible := 5
	if len(descLines) > maxVisible {
		descLines = descLines[len(descLines)-maxVisible:]
	}
	if len(descLines) == 0 {
		descLines = []string{""}
	}

	lines := []string{
		m.styles.PanelTitle.Render(title),
		"",
		fmt.Sprintf("%s Summary:     %s", msgIndicator, msgDisplay),
		"",
		fmt.Sprintf("%s Description:", descIndicator),
	}
	for _, dl := range descLines {
		lines = append(lines, fmt.Sprintf("  %s", dl))
	}
	var hint string
	if m.commitPromptField == commitFieldMessage {
		hint = "enter: commit | tab: switch field | esc: cancel"
	} else {
		hint = "tab: switch field | esc: cancel"
	}
	lines = append(lines,
		"",
		hint,
	)

	content := strings.Join(lines, "\n")
	return m.styles.Panel.Width(panelWidth).Render(content)
}

func (m *Model) renderBranchPrompt() string {
	if !m.branchPromptActive {
		return ""
	}

	panelWidth := 60
	if m.width > 0 && m.width-4 < panelWidth {
		panelWidth = m.width - 4
	}
	if panelWidth < 30 {
		panelWidth = 30
	}
	contentWidth := panelWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	repoCount := len(m.branchPromptRepos)
	title := fmt.Sprintf("Create branch in %d repos", repoCount)
	if repoCount == 1 && m.branchPromptRepos[0] != nil {
		title = fmt.Sprintf("Create branch in %s", truncateString(m.branchPromptRepos[0].Name, contentWidth-17))
	}

	branchDisplay := m.branchNameBuffer
	if len(branchDisplay) > contentWidth-2 {
		branchDisplay = branchDisplay[len(branchDisplay)-contentWidth+2:]
	}

	lines := []string{
		m.styles.PanelTitle.Render(title),
		"",
		fmt.Sprintf("> Branch: %s", branchDisplay),
		"",
		"enter: create | esc: cancel",
	}

	return m.styles.Panel.Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func (m *Model) renderWorktreePrompt() string {
	if !m.worktreePromptActive || m.worktreePromptRepo == nil {
		return ""
	}

	panelWidth := 60
	if m.width > 0 && m.width-4 < panelWidth {
		panelWidth = m.width - 4
	}
	if panelWidth < 30 {
		panelWidth = 30
	}
	contentWidth := panelWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	branchIndicator := " "
	pathIndicator := " "
	if m.worktreePromptField == worktreeFieldBranch {
		branchIndicator = ">"
	} else {
		pathIndicator = ">"
	}

	branchDisplay := m.worktreeBranchBuffer
	if len(branchDisplay) > contentWidth-2 {
		branchDisplay = branchDisplay[len(branchDisplay)-contentWidth+2:]
	}
	pathDisplay := m.worktreePathBuffer
	if len(pathDisplay) > contentWidth-2 {
		pathDisplay = pathDisplay[len(pathDisplay)-contentWidth+2:]
	}

	lines := []string{
		m.styles.PanelTitle.Render(fmt.Sprintf("Create worktree in %s", truncateString(m.worktreePromptRepo.Name, contentWidth-20))),
		"",
		fmt.Sprintf("%s Branch: %s", branchIndicator, branchDisplay),
		fmt.Sprintf("%s Path:   %s", pathIndicator, pathDisplay),
		"",
		"enter: next/create | tab: switch field | esc: cancel",
	}
	return m.styles.Panel.Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func splitDescLines(s string, maxWidth int) []string {
	if s == "" {
		return nil
	}
	raw := strings.Split(s, "\n")
	var result []string
	for _, line := range raw {
		if maxWidth > 0 && len(line) > maxWidth {
			line = line[len(line)-maxWidth:]
		}
		result = append(result, line)
	}
	return result
}

func (m *Model) renderCredentialPrompt() string {
	prompt := m.activeCredentialPrompt
	if prompt == nil {
		return ""
	}
	repoName := "repository"
	if prompt.repo != nil && prompt.repo.Name != "" {
		repoName = prompt.repo.Name
	}
	server := ""
	if prompt.repo != nil && prompt.repo.State != nil && prompt.repo.State.Remote != nil && len(prompt.repo.State.Remote.URL) > 0 {
		server = prompt.repo.State.Remote.URL[0]
		server = strings.TrimPrefix(server, "https://")
		server = strings.TrimPrefix(server, "http://")
		server = strings.TrimPrefix(server, "ssh://")
		server = strings.TrimPrefix(server, "git@")
		if idx := strings.Index(server, "/"); idx != -1 {
			server = server[:idx]
		}
		if idx := strings.Index(server, ":"); idx != -1 {
			server = server[:idx]
		}
	}

	panelWidth := m.width
	if panelWidth < 24 {
		panelWidth = 24
	}
	contentWidth := panelWidth - 4
	if contentWidth < 10 {
		contentWidth = 10
	}
	usernameDisplay := prompt.username
	if m.credentialInputField == credentialFieldUsername {
		usernameDisplay = m.credentialInputBuffer
	}
	passwordLen := len([]rune(prompt.password))
	if m.credentialInputField == credentialFieldPassword {
		passwordLen = len([]rune(m.credentialInputBuffer))
	}
	passwordDisplay := strings.Repeat("*", passwordLen)
	usernameIndicator := " "
	passwordIndicator := " "
	if m.credentialInputField == credentialFieldUsername {
		usernameIndicator = ">"
	} else {
		passwordIndicator = ">"
	}
	lines := []string{
		fmt.Sprintf("Credentials required for %s", truncateString(repoName, contentWidth)),
	}
	if server != "" {
		lines = append(lines, fmt.Sprintf("Server: %s", truncateString(server, contentWidth)))
	}
	lines = append(lines,
		"",
		fmt.Sprintf("%s Username: %s", usernameIndicator, truncateString(usernameDisplay, contentWidth-11)),
		fmt.Sprintf("%s Password: %s", passwordIndicator, truncateString(passwordDisplay, contentWidth-11)),
		"",
		"enter: submit | esc: cancel",
	)
	content := strings.Join(lines, "\n")
	return m.styles.Panel.Width(panelWidth).Render(content)
}
