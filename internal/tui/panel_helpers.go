package tui

import (
	"strings"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

type remoteBranchItem struct {
	remote *git.Remote
	branch *git.RemoteBranch
}

func remoteBranchItems(r *git.Repository) []remoteBranchItem {
	items := make([]remoteBranchItem, 0)
	if r == nil {
		return items
	}
	for _, remote := range r.Remotes {
		if remote == nil {
			continue
		}
		for _, branch := range remote.Branches {
			if branch == nil {
				continue
			}
			items = append(items, remoteBranchItem{remote: remote, branch: branch})
		}
	}
	return items
}

func remoteBranchDisplayName(item remoteBranchItem) string {
	return item.branch.Name
}

func remoteBranchShortName(item remoteBranchItem) string {
	name := item.branch.Name
	if item.remote != nil {
		prefix := item.remote.Name + "/"
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
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

func branchIndex(branches []*git.Branch, name string) int {
	for i, branch := range branches {
		if branch != nil && branch.Name == name {
			return i
		}
	}
	return -1
}

func remoteBranchIndex(items []remoteBranchItem, fullName string) int {
	for i, item := range items {
		if item.branch != nil && item.branch.Name == fullName {
			return i
		}
	}
	return -1
}
