package watch

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// pollEntry holds per-repository state for the polling watcher.
type pollEntry struct {
	repo   *git.Repository
	gitDir string
	mtimes map[string]time.Time
	timer  *time.Timer
}

type pollingWatcher struct {
	mu      sync.Mutex
	entries map[*git.Repository]*pollEntry
	ticker  *time.Ticker
	done    chan struct{}
	closed  bool
}

func newPollingWatcher() *pollingWatcher {
	return newPollingWatcherWithInterval(pollingInterval)
}

// newPollingWatcherWithInterval creates a polling watcher with a custom tick
// interval. Intended for use in tests where the default 2-second interval is
// too slow.
func newPollingWatcherWithInterval(interval time.Duration) *pollingWatcher {
	pw := &pollingWatcher{
		entries: make(map[*git.Repository]*pollEntry),
		ticker:  time.NewTicker(interval),
		done:    make(chan struct{}),
	}
	go pw.loop()
	return pw
}

func (pw *pollingWatcher) register(r *git.Repository) {
	if r == nil || r.AbsPath == "" {
		return
	}
	gitDir := filepath.Join(r.AbsPath, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return
	}

	// Stat initial file times before taking the lock to avoid I/O under lock.
	initial := make(map[string]time.Time, len(trackedGitFiles))
	for _, f := range trackedGitFiles {
		path := filepath.Join(gitDir, f)
		if fi, err := os.Stat(path); err == nil {
			initial[path] = fi.ModTime()
		}
	}

	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.closed {
		return
	}
	if _, ok := pw.entries[r]; ok {
		return // already registered
	}
	pw.entries[r] = &pollEntry{
		repo:   r,
		gitDir: gitDir,
		mtimes: initial,
	}
}

func (pw *pollingWatcher) close() error {
	pw.mu.Lock()
	if pw.closed {
		pw.mu.Unlock()
		return nil
	}
	pw.closed = true
	for _, e := range pw.entries {
		if e.timer != nil {
			e.timer.Stop()
		}
	}
	pw.ticker.Stop()
	pw.mu.Unlock()
	close(pw.done)
	return nil
}

func (pw *pollingWatcher) loop() {
	for {
		select {
		case <-pw.done:
			return
		case <-pw.ticker.C:
			pw.tick()
		}
	}
}

// tick snapshots the registered entries and checks each for file changes.
// The snapshot is captured under the lock but all I/O happens outside it
// to avoid blocking register and close during disk access.
func (pw *pollingWatcher) tick() {
	pw.mu.Lock()
	if pw.closed {
		pw.mu.Unlock()
		return
	}
	entries := make([]*pollEntry, 0, len(pw.entries))
	for _, e := range pw.entries {
		entries = append(entries, e)
	}
	pw.mu.Unlock()

	for _, e := range entries {
		pw.checkEntry(e)
	}
}

// checkEntry stats all tracked git files and schedules a debounced refresh if
// any modification time has advanced since the last poll. File I/O is done
// without holding the lock; only the mtime map update and change detection are
// locked.
func (pw *pollingWatcher) checkEntry(e *pollEntry) {
	type statResult struct {
		path  string
		mtime time.Time
	}

	// Stat files outside the lock.
	results := make([]statResult, 0, len(trackedGitFiles))
	for _, f := range trackedGitFiles {
		path := filepath.Join(e.gitDir, f)
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		results = append(results, statResult{path, fi.ModTime()})
	}

	// Update recorded mtimes under the lock and detect changes.
	pw.mu.Lock()
	if pw.closed {
		pw.mu.Unlock()
		return
	}
	changed := false
	for _, r := range results {
		last, seen := e.mtimes[r.path]
		if !seen {
			// First observation: record as baseline without triggering a refresh.
			e.mtimes[r.path] = r.mtime
			continue
		}
		if r.mtime.After(last) {
			e.mtimes[r.path] = r.mtime
			changed = true
		}
	}
	pw.mu.Unlock()

	if changed {
		pw.scheduleRefresh(e)
	}
}

func (pw *pollingWatcher) scheduleRefresh(e *pollEntry) {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.closed {
		return
	}
	if e.timer != nil {
		e.timer.Reset(fsnotifyDebounce)
		return
	}
	e.timer = time.AfterFunc(fsnotifyDebounce, func() {
		pw.mu.Lock()
		e.timer = nil
		closed := pw.closed
		pw.mu.Unlock()
		if closed || e.repo.WatchRefreshSuppressed() {
			return
		}
		command.RequestExternalRefresh(e.repo)
	})
}
