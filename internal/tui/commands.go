package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/load"
)

// repositoryLoadedMsg is sent when a single repository is loaded
type repositoryLoadedMsg struct {
	repo *git.Repository
}

// loadRepositoriesCmd returns a command that loads repositories asynchronously
func loadRepositoriesCmd(directories []string) tea.Cmd {
	return func() tea.Msg {
		// Use a channel to collect repositories
		repoChan := make(chan *git.Repository, 10)
		loaded := make(chan bool, 1)
		
		// Start loading in background
		go func() {
			_ = load.AsyncLoad(directories, func(r *git.Repository) {
				repoChan <- r
			}, loaded)
			<-loaded
			close(repoChan)
		}()
		
		// Collect all repositories
		var repos []*git.Repository
		for repo := range repoChan {
			repos = append(repos, repo)
		}
		
		if len(repos) == 0 {
			return errMsg{err: ErrNoRepositories}
		}
		
		return repositoriesLoadedMsg{repos: repos}
	}
}

// ErrNoRepositories is returned when no repositories are found
var ErrNoRepositories = &noReposError{}

type noReposError struct{}

func (e *noReposError) Error() string {
	return "no git repositories found in the specified directories"
}
