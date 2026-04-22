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

	// Write a file with a basename not in trackedGitFilesSet, directly under .git/.
	noisePath := filepath.Join(repo.AbsPath, ".git", "scratch")
	require.NoError(t, os.WriteFile(noisePath, []byte("x"), 0644))

	// Give the watcher time to deliver an event and its (filtered) handling
	// to complete. Status must remain Available.
	time.Sleep(fsnotifyDebounce + 500*time.Millisecond)
	require.Equal(t, git.Available, repo.WorkStatus(),
		"writes to unwatched basenames should not trigger refresh")
}

func TestFSWatcherSuppressesInternalRefreshWindow(t *testing.T) {
	helper := gittest.InitTestRepositoryFromLocal(t)
	defer helper.CleanUp(t)
	repo := helper.Repository
	repo.SetWorkStatus(git.Available)
	repo.BeginWatchSuppress()
	defer repo.EndWatchSuppress()

	fw, err := newFSWatcher()
	require.NoError(t, err)
	defer fw.close()

	fw.register(repo)

	headPath := filepath.Join(repo.AbsPath, ".git", "HEAD")
	content, err := os.ReadFile(headPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(headPath, content, 0644))

	time.Sleep(fsnotifyDebounce + 500*time.Millisecond)
	require.Equal(t, git.Available, repo.WorkStatus(),
		"writes during suppression window should not trigger refresh")
}

// TestPollingWatcherRegisterNonRepo verifies that a directory without .git is
// silently skipped by the polling watcher.
func TestPollingWatcherRegisterNonRepo(t *testing.T) {
	tmp := t.TempDir()
	repo := &git.Repository{
		AbsPath: tmp,
		State:   &git.RepositoryState{},
	}

	pw := newPollingWatcherWithInterval(50 * time.Millisecond)
	defer pw.close()

	pw.register(repo)

	pw.mu.Lock()
	defer pw.mu.Unlock()
	require.Empty(t, pw.entries, "no entries should be registered for non-git dir")
}

// TestPollingWatcherTriggersOnGitFile verifies the polling watcher detects a
// modification to .git/HEAD and schedules a refresh.
func TestPollingWatcherTriggersOnGitFile(t *testing.T) {
	helper := gittest.InitTestRepositoryFromLocal(t)
	defer helper.CleanUp(t)
	repo := helper.Repository
	repo.SetWorkStatus(git.Available)

	pw := newPollingWatcherWithInterval(50 * time.Millisecond)
	defer pw.close()

	pw.register(repo)

	headPath := filepath.Join(repo.AbsPath, ".git", "HEAD")
	content, err := os.ReadFile(headPath)
	require.NoError(t, err)
	// Ensure the mtime advances (some filesystems have 1-second resolution).
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(headPath, content, 0644))

	require.Eventually(t, func() bool {
		return repo.WorkStatus() == git.Pending
	}, fsnotifyDebounce+2*time.Second, 50*time.Millisecond,
		"polling watcher should detect .git/HEAD change and trigger refresh")
}

// TestPollingWatcherSuppressesInternalRefreshWindow verifies that writes
// during a suppression window do not trigger a refresh.
func TestPollingWatcherSuppressesInternalRefreshWindow(t *testing.T) {
	helper := gittest.InitTestRepositoryFromLocal(t)
	defer helper.CleanUp(t)
	repo := helper.Repository
	repo.SetWorkStatus(git.Available)
	repo.BeginWatchSuppress()
	defer repo.EndWatchSuppress()

	pw := newPollingWatcherWithInterval(50 * time.Millisecond)
	defer pw.close()

	pw.register(repo)

	headPath := filepath.Join(repo.AbsPath, ".git", "HEAD")
	content, err := os.ReadFile(headPath)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(headPath, content, 0644))

	time.Sleep(fsnotifyDebounce + 500*time.Millisecond)
	require.Equal(t, git.Available, repo.WorkStatus(),
		"writes during suppression window should not trigger refresh")
}

// TestPollingWatcherRegisterIdempotent verifies that registering the same repo
// twice is a safe no-op.
func TestPollingWatcherRegisterIdempotent(t *testing.T) {
	helper := gittest.InitTestRepositoryFromLocal(t)
	defer helper.CleanUp(t)

	pw := newPollingWatcherWithInterval(50 * time.Millisecond)
	defer pw.close()

	pw.register(helper.Repository)
	pw.register(helper.Repository)

	pw.mu.Lock()
	count := len(pw.entries)
	pw.mu.Unlock()
	require.Equal(t, 1, count, "double-register should result in exactly one entry")
}

// TestIsContainerEnvironmentEnvVars verifies detection via environment variables
// that are safe to set in tests.
func TestIsContainerEnvironmentEnvVars(t *testing.T) {
	t.Run("detected via KUBERNETES_SERVICE_HOST", func(t *testing.T) {
		t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
		require.True(t, isContainerEnvironment())
	})

	t.Run("detected via container env var", func(t *testing.T) {
		t.Setenv("container", "podman")
		require.True(t, isContainerEnvironment())
	})
}

// TestNewWatcherUsesPollingInContainer verifies the factory selects the polling
// watcher when a container environment is indicated by an environment variable.
func TestNewWatcherUsesPollingInContainer(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	w := newWatcher()
	defer w.close()
	_, ok := w.(*pollingWatcher)
	require.True(t, ok, "expected pollingWatcher when KUBERNETES_SERVICE_HOST is set")
}

// TestNewWatcherUsesFSNotifyOnNativeSystem verifies the factory selects the
// fsnotify watcher on a non-container system.
func TestNewWatcherUsesFSNotifyOnNativeSystem(t *testing.T) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("running inside Docker, fsnotify watcher may not be selected")
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		t.Skip("running inside a container, fsnotify watcher may not be selected")
	}
	if os.Getenv("container") != "" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		t.Skip("container environment variables are set")
	}

	w := newWatcher()
	defer w.close()
	_, ok := w.(*fsWatcher)
	require.True(t, ok, "expected fsWatcher on a native (non-container) system")
}
