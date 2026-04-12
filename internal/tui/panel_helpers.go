package tui

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

type branchPanelItem struct {
	Name      string
	IsCurrent bool
}

type remotePanelEntry struct {
	RemoteName string
	BranchName string
	FullName   string
}

func findBranchByName(repo *git.Repository, name string) *git.Branch {
	if repo == nil {
		return nil
	}
	for _, branch := range repo.Branches {
		if branch != nil && branch.Name == name {
			return branch
		}
	}
	return nil
}

func (m *Model) taggedRepositories() []*git.Repository {
	return slices.DeleteFunc(slices.Clone(m.repositories), func(repo *git.Repository) bool {
		if repo == nil {
			return true
		}
		return repo.WorkStatus() != git.Queued
	})
}

func (m *Model) hasMultipleTagged() bool {
	return len(m.taggedRepositories()) > 1
}

func (m *Model) branchPanelItems() []branchPanelItem {
	repos := m.panelRepositories()
	if len(repos) == 0 {
		return nil
	}
	currentName := ""
	if len(repos) == 1 {
		repo := repos[0]
		if repo != nil && repo.State != nil && repo.State.Branch != nil {
			currentName = repo.State.Branch.Name
		}
	}

	if len(repos) > 1 {
		names := commonBranchNames(repos)
		items := make([]branchPanelItem, 0, len(names))
		for _, name := range names {
			items = append(items, branchPanelItem{
				Name:      name,
				IsCurrent: name == currentName,
			})
		}
		return items
	}

	repo := repos[0]
	items := make([]branchPanelItem, 0, len(repo.Branches))
	for _, branch := range repo.Branches {
		name := "<unknown>"
		if branch != nil {
			name = branch.Name
		}
		items = append(items, branchPanelItem{
			Name:      name,
			IsCurrent: name == currentName,
		})
	}
	return items
}

func (m *Model) remotePanelItems() []remotePanelEntry {
	repos := m.panelRepositories()
	if len(repos) == 0 {
		return nil
	}
	if len(repos) > 1 {
		return commonRemoteEntries(repos)
	}
	return remoteEntriesForRepo(repos[0])
}

func (m *Model) panelRepositories() []*git.Repository {
	tagged := m.taggedRepositories()
	if len(tagged) > 0 {
		return tagged
	}
	if repo := m.currentRepository(); repo != nil {
		return []*git.Repository{repo}
	}
	return nil
}

func commonBranchNames(repos []*git.Repository) []string {
	if len(repos) == 0 {
		return nil
	}
	common := make(map[string]struct{})
	first := repos[0]
	if first != nil {
		for _, branch := range first.Branches {
			if branch != nil {
				common[branch.Name] = struct{}{}
			}
		}
	}
	for _, repo := range repos[1:] {
		present := make(map[string]struct{})
		if repo != nil {
			for _, branch := range repo.Branches {
				if branch != nil {
					present[branch.Name] = struct{}{}
				}
			}
		}
		for name := range common {
			if _, ok := present[name]; !ok {
				delete(common, name)
			}
		}
	}
	names := slices.Collect(maps.Keys(common))
	slices.Sort(names)
	return names
}

func remoteEntriesForRepo(repo *git.Repository) []remotePanelEntry {
	entries := make([]remotePanelEntry, 0)
	if repo == nil {
		return entries
	}
	for _, remote := range repo.Remotes {
		if remote == nil {
			continue
		}
		for _, branch := range remote.Branches {
			if branch == nil {
				continue
			}
			fullName := branch.Name
			shortName := strings.TrimPrefix(fullName, remote.Name+"/")
			entries = append(entries, remotePanelEntry{
				RemoteName: remote.Name,
				BranchName: shortName,
				FullName:   fullName,
			})
		}
	}
	slices.SortFunc(entries, compareRemoteEntries)
	return entries
}

func remoteEntryMap(repo *git.Repository) map[string]remotePanelEntry {
	result := make(map[string]remotePanelEntry)
	if repo == nil {
		return result
	}
	for _, remote := range repo.Remotes {
		if remote == nil {
			continue
		}
		for _, branch := range remote.Branches {
			if branch == nil {
				continue
			}
			fullName := branch.Name
			shortName := strings.TrimPrefix(fullName, remote.Name+"/")
			result[fullName] = remotePanelEntry{
				RemoteName: remote.Name,
				BranchName: shortName,
				FullName:   fullName,
			}
		}
	}
	return result
}

func commonRemoteEntries(repos []*git.Repository) []remotePanelEntry {
	if len(repos) == 0 {
		return nil
	}
	common := remoteEntryMap(repos[0])
	for _, repo := range repos[1:] {
		entries := remoteEntryMap(repo)
		for fullName := range common {
			if _, ok := entries[fullName]; !ok {
				delete(common, fullName)
			}
		}
	}
	result := slices.Collect(maps.Values(common))
	slices.SortFunc(result, compareRemoteEntries)
	return result
}

func compareRemoteEntries(a, b remotePanelEntry) int {
	if diff := cmp.Compare(a.RemoteName, b.RemoteName); diff != 0 {
		return diff
	}
	return cmp.Compare(a.BranchName, b.BranchName)
}

// --- Stash panel items ---

type stashPanelItem struct {
	Description string
	BranchName  string
	StashID     int
}

func stashItemsForRepo(repo *git.Repository) []stashPanelItem {
	if repo == nil {
		return nil
	}
	items := make([]stashPanelItem, 0, len(repo.Stasheds))
	for _, s := range repo.Stasheds {
		if s == nil {
			continue
		}
		items = append(items, stashPanelItem{
			Description: s.Description,
			BranchName:  s.BranchName,
			StashID:     s.StashID,
		})
	}
	return items
}

func commonStashItems(repos []*git.Repository) []stashPanelItem {
	if len(repos) == 0 {
		return nil
	}
	// Build set of descriptions from first repo
	type stashInfo struct {
		Description string
		BranchName  string
	}
	common := make(map[string]stashInfo)
	if repos[0] != nil {
		for _, s := range repos[0].Stasheds {
			if s != nil {
				common[s.Description] = stashInfo{
					Description: s.Description,
					BranchName:  s.BranchName,
				}
			}
		}
	}
	// Intersect with other repos
	for _, repo := range repos[1:] {
		present := make(map[string]struct{})
		if repo != nil {
			for _, s := range repo.Stasheds {
				if s != nil {
					present[s.Description] = struct{}{}
				}
			}
		}
		for desc := range common {
			if _, ok := present[desc]; !ok {
				delete(common, desc)
			}
		}
	}
	// Convert to sorted slice
	items := make([]stashPanelItem, 0, len(common))
	for _, info := range common {
		items = append(items, stashPanelItem{
			Description: info.Description,
			BranchName:  info.BranchName,
		})
	}
	slices.SortFunc(items, func(a, b stashPanelItem) int {
		return cmp.Compare(a.Description, b.Description)
	})
	return items
}

func (m *Model) stashActionPanelItems() []stashPanelItem {
	repos := m.stashActionRepos()
	if len(repos) == 0 {
		return nil
	}
	if len(repos) > 1 {
		return commonStashItems(repos)
	}
	return stashItemsForRepo(repos[0])
}
