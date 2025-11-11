package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

const (
	queuedSymbol  = "●"
	successSymbol = "✓"
	failSymbol    = "✗"
	dirtySymbol   = "⚠"

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

const (
	maxRepoDisplayWidth   = 40
	maxBranchDisplayWidth = 40
	commitColumnMinWidth  = 10
)

const panelHorizontalFrame = 4

func (m *Model) overviewTitleHeight() int {
	if m.height <= 0 {
		return 0
	}
	return 1
}

func (m *Model) overviewTableBodyHeight() int {
	visible := m.height - 4
	if visible < 0 {
		visible = 0
	}
	return visible + 2
}

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

func (m *Model) overviewTableWidth() int {
	if m.width <= 0 {
		return 0
	}
	widths := m.getColumnWidths()
	total := widths.repo + widths.branch + widths.commitMsg + 4
	if total > m.width {
		total = m.width
	}
	if total < 0 {
		total = 0
	}
	return total
}

func (m *Model) sidePanelMaxWidth() int {
	base := m.overviewTableWidth()
	if base <= 0 {
		return 0
	}
	half := m.width / 2
	if half > 0 && base > half {
		base = half
	}
	minWidth := panelHorizontalFrame + 12
	if minWidth > m.width && m.width > 0 {
		minWidth = m.width
	}
	if base < minWidth {
		base = minWidth
	}
	if base > m.width {
		base = m.width
	}
	return base
}

func (m *Model) sidePanelTopPadding() int {
	return m.overviewTitleHeight() + 1
}

func (m *Model) sidePanelLayoutDimensions(repo *git.Repository) (panelWidth int, gapWidth int, ok bool) {
	baseWidth := m.sidePanelMaxWidth()
	if baseWidth < panelHorizontalFrame+1 {
		return baseWidth, 0, false
	}
	mainWidth := 0
	if repo != nil {
		mainInfo := m.renderRepositoryInfo(repo)
		mainWidth = lipgloss.Width(mainInfo)
	}
	if mainWidth < 0 {
		mainWidth = 0
	}
	gapOptions := []int{2, 1, 0}
	for _, gap := range gapOptions {
		maxAvailable := m.width - mainWidth - gap
		if maxAvailable < panelHorizontalFrame+1 {
			continue
		}
		candidate := baseWidth
		if candidate > maxAvailable {
			candidate = maxAvailable
		}
		if candidate < panelHorizontalFrame+1 {
			continue
		}
		return candidate, gap, true
	}
	return baseWidth, 0, false
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

	if m.currentView == OverviewView {
		content = m.renderOverview()
	} else {
		content = m.renderFocus()
	}

	if m.showHelp {
		help := m.renderHelp()
		content = lipgloss.JoinVertical(lipgloss.Left, content, help)
	}

	if errorBanner != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, errorBanner, content)
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

	// Compute column widths based on content and available width (cached)
	colWidths := m.getColumnWidths()

	title := m.renderOverviewTitleBar()

	var topLabel, bottomLabel string
	if startIdx > 0 {
		topLabel = "↑ more above"
	}
	if endIdx < len(m.repositories) {
		bottomLabel = "↓ more below"
	}

	// Top border for table
	topBorder := m.renderTableBorder(colWidths, "top", topLabel)

	// Render repositories
	var lines []string
	for i := startIdx; i < endIdx; i++ {
		r := m.repositories[i]
		selected := i == m.cursor
		line := m.renderRepositoryLine(r, selected, colWidths)
		lines = append(lines, line)
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
	bottomBorder := m.renderTableBorder(colWidths, "bottom", bottomLabel)

	list := strings.Join(lines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, title, topBorder, list, bottomBorder)
}

// renderRepositoryLine renders a single repository line as a table row
// Table format: │cursor status repo-name    │ branch-name │ commit tags/message │
// Example:      │→ ●   example-repo         │  main       │ [v1.0.0] add feature │
func (m *Model) renderRepositoryLine(r *git.Repository, selected bool, colWidths columnWidths) string {
	statusIcon := " "
	style := m.styles.ListItem
	status := r.WorkStatus()
	dirty := repoIsDirty(r)
	failed := status == git.Fail
	recoverable := failed && r.State != nil && r.State.RecoverableError

	switch status {
	case git.Pending:
		statusIcon = waitingSymbol
		style = m.styles.PendingItem
	case git.Queued:
		statusIcon = queuedSymbol
		style = m.styles.QueuedItem
	case git.Working:
		if len(spinnerFrames) > 0 {
			statusIcon = spinnerFrames[m.spinnerIndex%len(spinnerFrames)]
		} else {
			statusIcon = "*"
		}
		style = m.styles.WorkingItem
	case git.Success:
		statusIcon = successSymbol
		style = m.styles.SuccessItem
	case git.Fail:
		statusIcon = failSymbol
		if recoverable {
			style = m.styles.RecoverableFailedItem
		} else {
			style = m.styles.FailedItem
		}
	}
	// Only show dirty symbol when state evaluation is complete (status.Ready).
	// During evaluation (Working), keep showing the spinner.
	if dirty && !failed && status.Ready {
		statusIcon = dirtySymbol
		style = m.styles.DirtyItem
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
	if dirty && !failed && !selected {
		repoColumn = m.styles.DirtyItem.Render(repoColumn)
	}

	branchContentWidth := colWidths.branch - 1
	if branchContentWidth < 0 {
		branchContentWidth = 0
	}
	branchContent := truncateString(branchContent(r), branchContentWidth)
	branchColumn := fmt.Sprintf("%-*s", colWidths.branch, " "+branchContent)
	if dirty && !failed && !selected {
		branchColumn = m.styles.DirtyItem.Render(branchColumn)
	}

	commitContentWidth := colWidths.commitMsg - 1
	if commitContentWidth < 0 {
		commitContentWidth = 0
	}

	fullCommitContent := commitContentForRepo(r)
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
	commitColumn := fmt.Sprintf("%-*s", colWidths.commitMsg, " "+commitContent)
	if dirty && !failed && !selected {
		commitColumn = m.styles.DirtyItem.Render(commitColumn)
	}

	var styledRepoCol, styledBranchCol, styledCommitCol string
	if selected {
		var highlight lipgloss.Style
		switch {
		case recoverable:
			highlight = m.styles.RecoverableFailedSelectedItem
		case failed:
			highlight = m.styles.FailedSelectedItem
		case dirty:
			highlight = m.styles.DirtySelectedItem
		default:
			highlight = m.styles.SelectedItem
		}
		styledRepoCol = highlight.Render(repoColumn)
		styledBranchCol = highlight.Render(branchColumn)
		styledCommitCol = highlight.Render(commitColumn)
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

func commitContentForRepo(r *git.Repository) string {
	if r == nil {
		return ""
	}
	if r.WorkStatus() == git.Fail && r.State != nil {
		message := singleLineMessage(r.State.Message)
		if message == "" {
			return "unknown error"
		}
		return message
	}

	commitMsg, commitHash := commitSummary(r)
	tags := collectTags(r, commitHash)
	parts := make([]string, 0, 2)
	if len(tags) > 0 {
		parts = append(parts, "["+strings.Join(tags, ", ")+"]")
	}
	if commitMsg != "" {
		parts = append(parts, commitMsg)
	}
	return strings.Join(parts, " ")
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

func commitPanelLineContent(commit *git.Commit) string {
	if commit == nil {
		return "(none)"
	}
	label := ""
	switch commit.CommitType {
	case git.LocalCommit:
		label = "[local] "
	case git.RemoteCommit:
		label = "[remote] "
	}
	hash := "(none)"
	if commit.Hash != "" {
		hash = shortHash(commit.Hash)
	}
	message := singleLineMessage(commit.Message)
	if message == "" {
		return strings.TrimSpace(label + hash)
	}
	return strings.TrimSpace(fmt.Sprintf("%s%s %s", label, hash, message))
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

// renderFocus renders the focus view with side panel
func (m *Model) renderFocus() string {
	if len(m.repositories) == 0 {
		return "No repository selected"
	}

	r := m.repositories[m.cursor]

	// Main info
	mainInfo := m.renderRepositoryInfo(r)

	panelWidth, gapWidth, horizontalOK := m.sidePanelLayoutDimensions(r)
	if panelWidth < panelHorizontalFrame+1 {
		horizontalOK = false
	}
	contentWidth := panelWidth - panelHorizontalFrame
	if contentWidth < 1 {
		contentWidth = 1
	}
	bodyHeight := m.height - m.overviewTitleHeight() - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	mainInfoHeight := lipgloss.Height(mainInfo)
	padLines := m.sidePanelTopPadding()
	panelFrameHeight := 3
	maxLines := bodyHeight - padLines - panelFrameHeight
	if !horizontalOK {
		available := bodyHeight - mainInfoHeight - padLines - panelFrameHeight
		if available < maxLines {
			maxLines = available
		}
	}
	if maxLines < 1 {
		maxLines = 1
	}

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
		panelContent = m.renderBranches(r, contentWidth, maxLines)
	case RemotePanel:
		if m.hasMultipleTagged() {
			panelTitle = "Common Remote Branches"
		} else {
			panelTitle = "Remotes"
		}
		panelContent = m.renderRemotes(r, contentWidth, maxLines)
	case CommitPanel:
		panelTitle = "Commits"
		panelContent = m.renderCommits(r, contentWidth, maxLines)
	case StatusPanel:
		panelTitle = "Status"
		panelContent = m.renderStatus(r, contentWidth, maxLines)
	case StashPanel:
		panelTitle = "Stash"
		panelContent = m.renderStash(r, contentWidth, maxLines)
	default:
		return mainInfo
	}

	panel := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.PanelTitle.Render(panelTitle),
		panelContent,
	)

	panelStyle := m.styles.Panel.Copy()
	if panelWidth > 0 {
		panelStyle = panelStyle.Width(panelWidth)
	}
	if (m.sidePanel == BranchPanel || m.sidePanel == RemotePanel) && m.hasMultipleTagged() {
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

	styledPanel := panelStyle.Render(panel)
	if pad := m.sidePanelTopPadding(); pad > 0 {
		styledPanel = strings.Repeat("\n", pad) + styledPanel
	}

	var combined string
	if !horizontalOK {
		combined = lipgloss.JoinVertical(lipgloss.Left, mainInfo, styledPanel)
	} else {
		gap := ""
		if gapWidth > 0 {
			gap = strings.Repeat(" ", gapWidth)
		}
		combined = lipgloss.JoinHorizontal(lipgloss.Top, mainInfo, gap, styledPanel)
	}

	title := m.renderOverviewTitleBar()
	if title != "" {
		return lipgloss.JoinVertical(lipgloss.Left, title, combined)
	}
	return combined
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
func (m *Model) renderBranches(r *git.Repository, contentWidth, maxLines int) string {
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
	instructions := fmt.Sprintf("%s checkout  %s delete",
		m.styles.KeyBinding.Render("[space]"),
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
func (m *Model) renderRemotes(r *git.Repository, contentWidth, maxLines int) string {
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
		m.styles.KeyBinding.Render("[space]"),
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

// renderCommits renders commit list
func (m *Model) renderCommits(r *git.Repository, contentWidth, maxLines int) string {
	if m.hasMultipleTagged() {
		return padToWidth("Commit view unavailable when multiple repositories are tagged", contentWidth)
	}

	if r == nil || r.State == nil || r.State.Branch == nil {
		return padToWidth("No branch selected", contentWidth)
	}

	commits := r.State.Branch.Commits

	count := len(commits)
	if count == 0 {
		return padToWidth("No commits", contentWidth)
	}

	if contentWidth <= 0 || maxLines <= 0 {
		return ""
	}

	viewport := m.commitViewportSize(count)
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

	lines := make([]string, 0, maxLines)
	instructions := fmt.Sprintf("%s checkout  %s soft reset  %s mixed reset  %s hard reset",
		m.styles.KeyBinding.Render("[space]"),
		m.styles.KeyBinding.Render("[s]"),
		m.styles.KeyBinding.Render("[m]"),
		m.styles.KeyBinding.Render("[h]"),
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
		lines = append(lines, padToWidth(m.styles.Help.Render("  ↑ more above"), contentWidth))
		remaining--
	}

	lineWidth := contentWidth

	for i := start; i < end; i++ {
		if remaining <= 0 {
			break
		}
		commit := commits[i]
		lineContent := commitPanelLineContent(commit)
		offset := m.getCommitDetailOffset(r, commit, i)
		maxOffset := maxCommitOffset(lineContent, lineWidth)
		if offset > maxOffset {
			offset = maxOffset
			m.setCommitDetailOffset(r, commit, i, offset)
		} else if maxOffset == 0 && offset != 0 {
			offset = 0
			m.setCommitDetailOffset(r, commit, i, 0)
		}
		visible := visibleCommitContent(lineContent, offset, lineWidth)
		if visible == "" {
			visible = fitStringToWidth(lineContent, lineWidth)
			if visible == "" {
				visible = lineContent
			}
		}
		line := visible
		if i == m.commitCursor {
			lines = append(lines, m.styles.SelectedItem.Render(padToWidth(line, contentWidth)))
		} else {
			lines = append(lines, padToWidth(line, contentWidth))
		}
		remaining--
	}

	if end < count {
		if remaining > 0 {
			lines = append(lines, padToWidth(m.styles.Help.Render("  ↓ more below"), contentWidth))
		}
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
	line := "On branch " + m.styles.BranchInfo.Render(r.State.Branch.Name)
	lines = append(lines, padToWidth(line, contentWidth))

	pushables, _ := strconv.Atoi(r.State.Branch.Pushables)
	pullables, _ := strconv.Atoi(r.State.Branch.Pullables)

	if len(lines) >= maxLines {
		return strings.Join(clampLines(lines, maxLines), "\n")
	}

	switch {
	case r.State.Branch.Upstream == nil:
		lines = append(lines, padToWidth("Not tracking a remote branch", contentWidth))
	case pushables == 0 && pullables == 0:
		message := "Up to date with " + m.styles.BranchInfo.Render(r.State.Branch.Upstream.Name)
		lines = append(lines, padToWidth(message, contentWidth))
	default:
		if pushables > 0 && pullables > 0 {
			lines = append(lines, padToWidth(fmt.Sprintf("Diverged from %s", r.State.Branch.Upstream.Name), contentWidth))
		} else if pushables > 0 {
			lines = append(lines, padToWidth(fmt.Sprintf("Ahead by %d commit(s)", pushables), contentWidth))
		} else {
			lines = append(lines, padToWidth(fmt.Sprintf("Behind by %d commit(s)", pullables), contentWidth))
		}
	}

	lines = clampLines(lines, maxLines)
	return strings.Join(lines, "\n")
}

// renderStash renders stash list
func (m *Model) renderStash(r *git.Repository, contentWidth, maxLines int) string {
	if contentWidth <= 0 || maxLines <= 0 {
		return ""
	}

	stashes := r.Stasheds
	if len(stashes) == 0 {
		return padToWidth("No stashes", contentWidth)
	}

	lines := make([]string, 0, maxLines)
	for _, stash := range stashes {
		line := fmt.Sprintf("stash@{%d}: %s %s", stash.StashID, stash.BranchName, stash.Description)
		lines = append(lines, padToWidth(line, contentWidth))
		if len(lines) >= maxLines {
			break
		}
	}

	lines = clampLines(lines, maxLines)
	return strings.Join(lines, "\n")
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
	dirty := repoIsDirty(focusRepo)
	failed := focusRepo != nil && focusRepo.WorkStatus() == git.Fail
	recoverable := failed && focusRepo.State != nil && focusRepo.State.RecoverableError

	center := ""

	right := "TAB: lazygit | ? for help"

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)

	if center == "" {
		tagHint := "space: tag for batch run"
		if focusRepo != nil && focusRepo.WorkStatus() == git.Queued {
			tagHint = "space: untag"
		}
		if queuedCount > 0 {
			tagHint += " | enter: batch run"
			center = fmt.Sprintf("tagged: %d | %s", queuedCount, tagHint)
		} else if m.activeForcePrompt == nil && m.activeCredentialPrompt == nil {
			center = "f fetch | p pull | P push | space: tag for batch run"
		}
	}

	if center == "" && m.activeCredentialPrompt != nil {
		statusBarStyle = m.styles.StatusBarMerge
		repoName := "credentials"
		if m.activeCredentialPrompt.repo != nil {
			repoName = truncateString(m.activeCredentialPrompt.repo.Name, 20)
		}
		left = fmt.Sprintf(" auth required: %s", repoName)

		label := "Username"
		display := m.credentialInputBuffer
		if m.credentialInputField == credentialFieldPassword {
			label = "Password"
			display = strings.Repeat("*", utf8.RuneCountInString(m.credentialInputBuffer))
		}

		right = "enter: submit"
		maxCenter := totalWidth - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if maxCenter < 0 {
			maxCenter = 0
		}
		center = truncateString(fmt.Sprintf("%s: %s", label, display), maxCenter)
	} else {
		if failed {
			message := "Operation failed"
			if focusRepo != nil && focusRepo.State != nil && focusRepo.State.Message != "" {
				message = truncateString(singleLineMessage(focusRepo.State.Message), totalWidth)
			}
			if recoverable {
				statusBarStyle = m.styles.StatusBarRecoverable
				left = " repo needs attention"
				right = "c: clear | TAB: lazygit"
				rightWidth = lipgloss.Width(right)
				maxCenter := totalWidth - lipgloss.Width(left) - rightWidth - 2
				if maxCenter < 0 {
					maxCenter = 0
				}
				center = truncateString(message, maxCenter)
			} else {
				statusBarStyle = m.styles.StatusBarError
				left = " repo failed"
				right = "c: clear"
				rightWidth = lipgloss.Width(right)
				maxCenter := totalWidth - lipgloss.Width(left) - rightWidth - 2
				if maxCenter < 0 {
					maxCenter = 0
				}
				center = truncateString(message, maxCenter)
			}
		} else if dirty {
			statusBarStyle = m.styles.StatusBarDirty
			left = " repo dirty"
			center = "Only TAB (lazygit) permitted while working tree is dirty"
			right = "TAB: lazygit"
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
		} else if !failed && !dirty && m.err == nil {
			center = fmt.Sprintf("%s", center)
		}
	}

	if m.currentView != OverviewView && m.activeCredentialPrompt == nil && m.activeForcePrompt == nil {
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
Navigation:  ↑/k up   g/Home top        ↓/j down G/End bottom
             PgUp/Ctrl+B page up        PgDn/Ctrl+F page down
             Ctrl+U half page up        Ctrl+D half page down

Actions:     Space   toggle queue       Enter   start queue
			 a       tag all            A       untag all
             m       cycle mode         Tab     open lazygit

Views:       b  branches    c  commits    r  remotes
             s  status      S  stash      ESC back (from views)

Sorting:     n  by name     t  by time

Git:         f  fetch repo   p  pull repo   P  push repo
Other:       ?  help         q/Ctrl+C  quit
`

	return m.styles.Help.Render(help)
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
		"",
		fmt.Sprintf("%s Username: %s", usernameIndicator, truncateString(usernameDisplay, contentWidth-11)),
		fmt.Sprintf("%s Password: %s", passwordIndicator, truncateString(passwordDisplay, contentWidth-11)),
		"",
		"enter: submit | tab: switch field | esc: cancel",
	}
	content := strings.Join(lines, "\n")
	return m.styles.Panel.Width(panelWidth).Render(content)
}
