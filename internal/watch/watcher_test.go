package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

// TestFSWatcherRegisterNonRepo verifies non-directory .git (worktrees) is
// silently skipped, and a missing .git is also tolerated.
func TestFSWatcherRegisterNonRepo(t *testing.T) {
	tmp := t.TempDir()
	repo := &git.Repository{
		AbsPath: tmp,
		State:   &git.RepositoryState{},
	}

	fw, err := newFSWatcher()
	require.NoError(t, err)
	defer fw.close()

	fw.register(repo) // no .git — should not panic and should not register

	fw.mu.Lock()
	defer fw.mu.Unlock()
	require.Empty(t, fw.byDir, "no watches should be registered for non-git dir")
}

// TestFSWatcherTriggersOnGitFile uses a real test repo and verifies the
// watcher schedules a refresh after a write to a watched file.
func TestFSWatcherTriggersOnGitFile(t *testing.T) {
	helper := gittest.InitTestRepositoryFromLocal(t)
	defer helper.CleanUp(t)
	repo := helper.Repository
	repo.SetWorkStatus(git.Available)

	fw, err := newFSWatcher()
	require.NoError(t, err)
	defer fw.close()

	fw.register(repo)

	// Touch .git/HEAD by rewriting its content with the same value.
	headPath := filepath.Join(repo.AbsPath, ".git", "HEAD")
	content, err := os.ReadFile(headPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(headPath, content, 0644))

	// Wait past the debounce + a refresh dispatch.
	require.Eventually(t, func() bool {
		return repo.WorkStatus() == git.Pending
	}, fsnotifyDebounce+2*time.Second, 50*time.Millisecond,
		"writing .git/HEAD should trigger a Pending refresh")
}

// TestFSWatcherFiltersIrrelevantFiles verifies writes to .git/objects/* (or
// other unwatched basenames) do not trigger a refresh.
func TestFSWatcherFiltersIrrelevantFiles(t *testing.T) {
	helper := gittest.InitTestRepositoryFromLocal(t)
	defer helper.CleanUp(t)
	repo := helper.Repository
	repo.SetWorkStatus(git.Available)

	fw, err := newFSWatcher()
	require.NoError(t, err)
	defer fw.close()
	fw.register(repo)

	// Write a file with a basename not in gitFiles, directly under .git/.
	noisePath := filepath.Join(repo.AbsPath, ".git", "scratch")
	require.NoError(t, os.WriteFile(noisePath, []byte("x"), 0644))

	// Give the watcher time to deliver an event and its (filtered) handling
	// to complete. Status must remain Available.
	time.Sleep(fsnotifyDebounce + 500*time.Millisecond)
	require.Equal(t, git.Available, repo.WorkStatus(),
		"writes to unwatched basenames should not trigger refresh")
}
