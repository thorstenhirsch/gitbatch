package tui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

type overviewRowKind uint8

const (
	overviewRepositoryRow overviewRowKind = iota
	overviewWorktreeHeaderRow
	overviewWorktreeRow
)

type overviewRow struct {
	kind        overviewRowKind
	repo        *git.Repository
	displayRepo *git.Repository
	worktree    *git.Worktree
}

func (r overviewRow) selectable() bool {
	return r.kind != overviewWorktreeHeaderRow
}

func (r overviewRow) repository() *git.Repository {
	if r.displayRepo != nil {
		return r.displayRepo
	}
	return r.repo
}

func (r overviewRow) actionRepository() *git.Repository {
	if r.displayRepo != nil {
		return r.displayRepo
	}
	return r.repo
}

func (r overviewRow) worktreeLabel() string {
	if r.worktree == nil {
		return "[main]"
	}
	if r.worktree.IsPrimary {
		return "[main]"
	}
	if r.worktree.IsDetached {
		head := r.worktree.Head
		if len(head) > 7 {
			head = head[:7]
		}
		if head == "" {
			head = "?"
		}
		return "(detached:" + head + ")"
	}
	return trimWorktreeRepositoryPrefix(r.worktree.DisplayName(), r.repo)
}

func (r overviewRow) worktreeStateMarkers() string {
	if r.worktree == nil {
		return ""
	}
	var markers []string
	if r.worktree.IsLocked {
		markers = append(markers, "L")
	}
	if r.worktree.IsPrunable {
		markers = append(markers, "P")
	}
	if len(markers) == 0 {
		return ""
	}
	return " [" + strings.Join(markers, ",") + "]"
}

func trimWorktreeRepositoryPrefix(label string, repo *git.Repository) string {
	if repo == nil {
		return label
	}
	name := strings.TrimSpace(repo.Name)
	if name == "" {
		return label
	}
	trimmed, ok := strings.CutPrefix(label, name)
	if !ok {
		return label
	}
	trimmed = strings.TrimLeft(trimmed, "-._ ")
	if trimmed == "" {
		return label
	}
	return trimmed
}

type worktreeFamily struct {
	key           string
	repo          *git.Repository
	members       []*git.Repository
	worktrees     []*git.Worktree
	worktreeRepos map[string]*git.Repository
}

func (f *worktreeFamily) repositoryForWorktree(worktree *git.Worktree) *git.Repository {
	if f == nil || worktree == nil {
		return nil
	}
	if repo := f.worktreeRepos[normalizeOverviewPath(worktree.Path)]; repo != nil {
		return repo
	}
	return nil
}

func (m *Model) overviewRows() []overviewRow {
	if !m.worktreeMode {
		rows := make([]overviewRow, 0, len(m.repositories))
		for _, repo := range m.repositories {
			if repo == nil {
				continue
			}
			rows = append(rows, overviewRow{
				kind:        overviewRepositoryRow,
				repo:        repo,
				displayRepo: repo,
			})
		}
		return rows
	}

	families := m.worktreeFamilies()
	rows := make([]overviewRow, 0)
	for _, family := range families {
		if family.repo == nil {
			continue
		}
		if len(family.worktrees) <= 1 {
			rows = append(rows, overviewRow{
				kind:        overviewRepositoryRow,
				repo:        family.repo,
				displayRepo: family.repo,
			})
			continue
		}
		for _, worktree := range family.worktrees {
			displayRepo := family.repositoryForWorktree(worktree)
			if displayRepo == nil {
				displayRepo = family.repo
			}
			rows = append(rows, overviewRow{
				kind:        overviewWorktreeRow,
				repo:        family.repo,
				displayRepo: displayRepo,
				worktree:    worktree,
			})
		}
	}
	return rows
}

func (m *Model) worktreeFamilies() []worktreeFamily {
	familyOrder := make([]string, 0)
	familyByKey := make(map[string]*worktreeFamily)

	for _, repo := range m.repositories {
		if repo == nil {
			continue
		}
		key := repo.FamilyKey()
		family := familyByKey[key]
		if family == nil {
			family = &worktreeFamily{
				key:           key,
				repo:          repo,
				worktreeRepos: make(map[string]*git.Repository),
			}
			familyByKey[key] = family
			familyOrder = append(familyOrder, key)
		}

		family.members = append(family.members, repo)
		if len(repo.Worktrees) > len(family.worktrees) {
			family.worktrees = slices.Clone(repo.Worktrees)
		}

		if current := repo.CurrentWorktree(); current != nil {
			family.worktreeRepos[normalizeOverviewPath(current.Path)] = repo
		}

		if primary := repo.PrimaryWorktree(); primary != nil && normalizeOverviewPath(primary.Path) == normalizeOverviewPath(repo.AbsPath) {
			family.repo = repo
		}
	}

	families := make([]worktreeFamily, 0, len(familyOrder))
	for _, key := range familyOrder {
		family := familyByKey[key]
		if family == nil {
			continue
		}
		if len(family.worktrees) == 0 && family.repo != nil {
			family.worktrees = slices.Clone(family.repo.Worktrees)
		}
		families = append(families, *family)
	}
	return families
}

func (m *Model) currentOverviewRow() (overviewRow, bool) {
	rows := m.overviewRows()
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return overviewRow{}, false
	}
	return rows[m.cursor], true
}

func (m *Model) overviewRowCount() int {
	return len(m.overviewRows())
}

func (m *Model) firstSelectableIndex() int {
	rows := m.overviewRows()
	for i, row := range rows {
		if row.selectable() {
			return i
		}
	}
	return 0
}

func (m *Model) lastSelectableIndex() int {
	rows := m.overviewRows()
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].selectable() {
			return i
		}
	}
	return 0
}

func (m *Model) moveOverviewCursor(delta int) {
	rows := m.overviewRows()
	if len(rows) == 0 {
		m.cursor = 0
		return
	}
	if delta == 0 {
		m.cursor = m.closestSelectableIndex(m.cursor, 1)
		return
	}
	idx := clampIndex(m.cursor, len(rows))
	for range len(rows) {
		idx += delta
		for idx < 0 {
			idx += len(rows)
		}
		for idx >= len(rows) {
			idx -= len(rows)
		}
		if rows[idx].selectable() {
			m.cursor = idx
			return
		}
	}
	m.cursor = idx
}

func (m *Model) closestSelectableIndex(start, direction int) int {
	rows := m.overviewRows()
	if len(rows) == 0 {
		return 0
	}
	start = clampIndex(start, len(rows))
	if rows[start].selectable() {
		return start
	}
	if direction == 0 {
		direction = 1
	}
	idx := start
	for range len(rows) {
		idx += direction
		if idx < 0 || idx >= len(rows) {
			break
		}
		if rows[idx].selectable() {
			return idx
		}
	}
	idx = start
	for range len(rows) {
		idx -= direction
		if idx < 0 || idx >= len(rows) {
			break
		}
		if rows[idx].selectable() {
			return idx
		}
	}
	return start
}

func (m *Model) toggleWorktreeMode() {
	selectedRepo := m.currentRepository()
	selectedWorktreePath := ""
	if row, ok := m.currentOverviewRow(); ok && row.worktree != nil {
		selectedWorktreePath = normalizeOverviewPath(row.worktree.Path)
	} else if selectedRepo != nil {
		if current := selectedRepo.CurrentWorktree(); current != nil {
			selectedWorktreePath = normalizeOverviewPath(current.Path)
		}
	}

	m.worktreeMode = !m.worktreeMode
	if !m.worktreeMode {
		if selectedRepo != nil {
			for i, repo := range m.repositories {
				if repo == selectedRepo {
					m.cursor = i
					return
				}
			}
		}
		m.cursor = clampIndex(m.cursor, len(m.repositories))
		return
	}

	rows := m.overviewRows()
	for i, row := range rows {
		if !row.selectable() {
			continue
		}
		if selectedWorktreePath != "" && row.worktree != nil && normalizeOverviewPath(row.worktree.Path) == selectedWorktreePath {
			m.cursor = i
			return
		}
		if selectedRepo != nil && row.actionRepository() == selectedRepo {
			m.cursor = i
			return
		}
	}
	m.cursor = m.firstSelectableIndex()
}

func normalizeOverviewPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !filepath.IsAbs(trimmed) {
		if abs, err := filepath.Abs(trimmed); err == nil {
			trimmed = abs
		}
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(trimmed)
}
