package tui

import (
	"fmt"
	"strings"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// clampIndex returns idx clamped to [0, length-1], or 0 if length == 0.
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

// wrapCursor adds delta to *cursor and wraps it within [0, length).
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

// ensureCursorVisible adjusts cursor and offset so the cursor stays within
// [0, total) and is visible inside the viewport window.
func ensureCursorVisible(cursor *int, offset *int, total, viewport int) {
	if total <= 0 {
		*cursor = 0
		*offset = 0
		return
	}
	if *cursor < 0 {
		*cursor = 0
	}
	if *cursor >= total {
		*cursor = total - 1
	}
	if viewport <= 0 {
		*offset = 0
		return
	}
	if *offset < 0 {
		*offset = 0
	}
	maxOffset := total - viewport
	if maxOffset < 0 {
		maxOffset = 0
	}
	if *offset > maxOffset {
		*offset = maxOffset
	}
	if *cursor < *offset {
		*offset = *cursor
	} else if *cursor >= *offset+viewport {
		*offset = *cursor - viewport + 1
	}
}

func (m *Model) ensureBranchCursorVisible(total, viewport int) {
	ensureCursorVisible(&m.branchCursor, &m.branchOffset, total, viewport)
}

func (m *Model) ensureRemoteCursorVisible(total, viewport int) {
	ensureCursorVisible(&m.remoteBranchCursor, &m.remoteOffset, total, viewport)
}

func (m *Model) ensureCommitCursorVisible(total, viewport int) {
	if viewport <= 0 {
		viewport = 1
	}
	if viewport > total {
		viewport = total
	}
	ensureCursorVisible(&m.commitCursor, &m.commitOffset, total, viewport)
}

// panelLineBudget returns the number of lines available for panel content.
func (m *Model) panelLineBudget() int {
	_, maxContentLines := m.popupDimensions()
	// Reserve lines for: panel title, repo header, blank separator
	budget := maxContentLines - 3
	if budget < 0 {
		budget = 0
	}
	return budget
}

func (m *Model) panelViewportSize(total int) int {
	budget := m.panelLineBudget()
	if budget <= 0 {
		return 0
	}
	remaining := budget - 1 // reserve one line for instructions
	if remaining <= 0 {
		return 0
	}
	if remaining > 1 && total > 0 {
		remaining--
	}
	if remaining < 0 {
		remaining = 0
	}
	if remaining > total {
		remaining = total
	}
	return remaining
}

func (m *Model) commitViewportSize(total int) int { return m.panelViewportSize(total) }
func (m *Model) branchViewportSize(total int) int { return m.panelViewportSize(total) }
func (m *Model) remoteViewportSize(total int) int { return m.panelViewportSize(total) }

// --- Commit row scroll (horizontal scrolling of the commit column in overview) ---

func (m *Model) getCommitScrollOffset(repo *git.Repository) int {
	if repo == nil || repo.RepoID == "" {
		return 0
	}
	return m.commitScrollOffsets[repo.RepoID]
}

func (m *Model) setCommitScrollOffset(repo *git.Repository, offset int) {
	if repo == nil || repo.RepoID == "" {
		return
	}
	if offset <= 0 {
		delete(m.commitScrollOffsets, repo.RepoID)
		return
	}
	m.commitScrollOffsets[repo.RepoID] = offset
}

func (m *Model) resetCommitScrollForSelected() {
	repo := m.currentRepository()
	if repo == nil {
		m.clearStaleGlobalError(nil)
		return
	}
	delete(m.commitScrollOffsets, repo.RepoID)
	m.clearStaleGlobalError(repo)
}

func (m *Model) clearStaleGlobalError(repo *git.Repository) {
	if m.err == nil {
		return
	}
	if repo == nil || repo.WorkStatus() != git.Fail {
		m.err = nil
	}
}

func (m *Model) adjustCommitScroll(delta int) bool {
	if delta == 0 {
		return false
	}
	repo := m.currentRepository()
	if repo == nil {
		return false
	}
	colWidths := calculateColumnWidths(m.width, m.repositories)
	contentWidth := colWidths.commitMsg - 1
	if contentWidth <= 0 {
		return false
	}
	content := commitContentForRepo(repo)
	maxOffset := maxCommitOffset(content, contentWidth)
	old := m.getCommitScrollOffset(repo)
	if maxOffset <= 0 {
		if old != 0 {
			m.setCommitScrollOffset(repo, 0)
			return true
		}
		return false
	}
	offset := old + delta
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset == old {
		return false
	}
	m.setCommitScrollOffset(repo, offset)
	return true
}

// --- Commit detail scroll (vertical scrolling inside the commit panel) ---

func (m *Model) repoKey(repo *git.Repository) string {
	if repo == nil {
		return ""
	}
	if repo.RepoID != "" {
		return repo.RepoID
	}
	if repo.AbsPath != "" {
		return repo.AbsPath
	}
	return repo.Name
}

func (m *Model) commitDetailKey(repo *git.Repository, commit *git.Commit, index int) string {
	key := m.repoKey(repo)
	if key == "" {
		return ""
	}
	if commit != nil && commit.Hash != "" {
		return key + ":" + commit.Hash
	}
	return fmt.Sprintf("%s:idx:%d", key, index)
}

func (m *Model) getCommitDetailOffset(repo *git.Repository, commit *git.Commit, index int) int {
	if m.commitDetailScroll == nil {
		return 0
	}
	key := m.commitDetailKey(repo, commit, index)
	if key == "" {
		return 0
	}
	return m.commitDetailScroll[key]
}

func (m *Model) setCommitDetailOffset(repo *git.Repository, commit *git.Commit, index int, offset int) {
	key := m.commitDetailKey(repo, commit, index)
	if key == "" {
		return
	}
	if offset <= 0 {
		if m.commitDetailScroll != nil {
			delete(m.commitDetailScroll, key)
		}
		return
	}
	if m.commitDetailScroll == nil {
		m.commitDetailScroll = make(map[string]int)
	}
	m.commitDetailScroll[key] = offset
}

func (m *Model) resetCommitDetailOffset(repo *git.Repository, commit *git.Commit, index int) {
	if m.commitDetailScroll == nil {
		return
	}
	key := m.commitDetailKey(repo, commit, index)
	if key == "" {
		return
	}
	delete(m.commitDetailScroll, key)
}

func (m *Model) resetCommitDetailScrollForIndex(repo *git.Repository, commits []*git.Commit, index int) {
	if repo == nil || len(commits) == 0 || index < 0 || index >= len(commits) {
		return
	}
	m.resetCommitDetailOffset(repo, commits[index], index)
}

func (m *Model) resetAllCommitDetailScroll(repo *git.Repository) {
	if repo == nil || m.commitDetailScroll == nil {
		return
	}
	prefix := m.repoKey(repo) + ":"
	if prefix == ":" {
		return
	}
	for key := range m.commitDetailScroll {
		if strings.HasPrefix(key, prefix) {
			delete(m.commitDetailScroll, key)
		}
	}
}

func (m *Model) commitPanelContentWidth() int {
	popupWidth, _ := m.popupDimensions()
	contentWidth := popupWidth - panelHorizontalFrame
	if contentWidth < 1 {
		contentWidth = 1
	}
	return contentWidth
}

func (m *Model) adjustCommitDetailScroll(delta int) bool {
	if delta == 0 {
		return false
	}
	repo := m.currentRepository()
	if repo == nil || repo.State == nil || repo.State.Branch == nil {
		return false
	}
	commits := repo.State.Branch.Commits
	if len(commits) == 0 {
		return false
	}
	index := clampIndex(m.commitCursor, len(commits))
	commit := commits[index]
	content := commitPanelLineContent(commit)
	width := m.commitPanelContentWidth()
	if width <= 0 {
		return false
	}
	maxOffset := maxCommitOffset(content, width)
	old := m.getCommitDetailOffset(repo, commit, index)
	if maxOffset <= 0 {
		if old != 0 {
			m.resetCommitDetailOffset(repo, commit, index)
			return true
		}
		return false
	}
	offset := old + delta
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset == old {
		return false
	}
	m.setCommitDetailOffset(repo, commit, index, offset)
	return true
}
