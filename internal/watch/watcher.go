package watch

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// fsnotifyDebounce coalesces the 3–5 file writes a single git operation
// produces (HEAD + index + refs/heads/<branch> + maybe packed-refs).
const fsnotifyDebounce = 500 * time.Millisecond

// gitFiles is the basename allow-list for events directly under .git/. Events
// on anything else (objects/, logs/, hooks/) are noise for our state model.
var gitFiles = map[string]struct{}{
	"HEAD":        {},
	"index":       {},
	"FETCH_HEAD":  {},
	"ORIG_HEAD":   {},
	"MERGE_HEAD":  {},
	"packed-refs": {},
	"config":      {},
}

type watched struct {
	repo   *git.Repository
	gitDir string
	timer  *time.Timer
}

type fsWatcher struct {
	w       *fsnotify.Watcher
	mu      sync.Mutex
	byDir   map[string]*watched
	byRepo  map[*git.Repository]*watched
	closeCh chan struct{}
	closed  bool
}

func newFSWatcher() (*fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fw := &fsWatcher{
		w:       w,
		byDir:   make(map[string]*watched),
		byRepo:  make(map[*git.Repository]*watched),
		closeCh: make(chan struct{}),
	}
	go fw.loop()
	return fw, nil
}

func (fw *fsWatcher) register(r *git.Repository) {
	if r == nil || r.AbsPath == "" {
		return
	}
	gitDir := filepath.Join(r.AbsPath, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		// Worktrees and submodules use a .git file pointing elsewhere — skipped for v1.
		return
	}

	dirs := []string{gitDir, filepath.Join(gitDir, "refs", "heads")}
	if entries, err := os.ReadDir(filepath.Join(gitDir, "refs", "remotes")); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(gitDir, "refs", "remotes", e.Name()))
			}
		}
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.closed {
		return
	}
	entry, ok := fw.byRepo[r]
	if !ok {
		entry = &watched{repo: r, gitDir: gitDir}
		fw.byRepo[r] = entry
	}
	for _, d := range dirs {
		if _, already := fw.byDir[d]; already {
			continue
		}
		if err := fw.w.Add(d); err != nil {
			// refs/heads may not exist yet on a brand-new repo — silently skip.
			continue
		}
		fw.byDir[d] = entry
	}
}

func (fw *fsWatcher) close() error {
	fw.mu.Lock()
	if fw.closed {
		fw.mu.Unlock()
		return nil
	}
	fw.closed = true
	for _, e := range fw.byRepo {
		if e.timer != nil {
			e.timer.Stop()
		}
	}
	fw.mu.Unlock()
	close(fw.closeCh)
	return fw.w.Close()
}

func (fw *fsWatcher) loop() {
	for {
		select {
		case <-fw.closeCh:
			return
		case ev, ok := <-fw.w.Events:
			if !ok {
				return
			}
			fw.handle(ev)
		case err, ok := <-fw.w.Errors:
			if !ok {
				return
			}
			log.Printf("watch: fsnotify error: %v", err)
		}
	}
}

func (fw *fsWatcher) handle(ev fsnotify.Event) {
	if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
		return
	}

	dir := filepath.Dir(ev.Name)
	base := filepath.Base(ev.Name)

	fw.mu.Lock()
	entry, ok := fw.byDir[dir]
	fw.mu.Unlock()
	if !ok {
		return
	}

	// Top-level .git/ events: filter to the basenames we care about. Subdir
	// events (refs/heads, refs/remotes/*) accept any change.
	if dir == entry.gitDir {
		if _, want := gitFiles[base]; !want {
			return
		}
	} else if ev.Op&fsnotify.Create != 0 {
		// New subdir under a watched refs tree (e.g. refs/heads/feature/)
		// — extend the watch so nested refs don't silently miss updates.
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			fw.addDir(ev.Name, entry)
		}
	}

	// .lock files appear transiently during ref updates; the real write that
	// follows triggers us properly.
	if strings.HasSuffix(base, ".lock") {
		return
	}

	fw.scheduleRefresh(entry)
}

func (fw *fsWatcher) addDir(dir string, entry *watched) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.closed {
		return
	}
	if _, already := fw.byDir[dir]; already {
		return
	}
	if err := fw.w.Add(dir); err != nil {
		return
	}
	fw.byDir[dir] = entry
}

func (fw *fsWatcher) scheduleRefresh(entry *watched) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.closed {
		return
	}
	if entry.timer != nil {
		entry.timer.Reset(fsnotifyDebounce)
		return
	}
	entry.timer = time.AfterFunc(fsnotifyDebounce, func() {
		fw.mu.Lock()
		entry.timer = nil
		fw.mu.Unlock()
		if entry.repo.WatchRefreshSuppressed() {
			return
		}
		command.RequestExternalRefresh(entry.repo)
	})
}
