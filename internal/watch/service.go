// Package watch detects external git operations and triggers a repository refresh.
//
// On native systems, changes are detected via fsnotify (inotify/kqueue) watches
// on each repository's .git/ directory. In container environments where
// filesystem event delivery is unreliable (e.g., Docker volumes, Podman mounts),
// the package transparently uses a polling-based watcher that checks file
// modification times on a fixed interval.
//
// Working-tree edits (unstaged changes to tracked files, new untracked files)
// are not detected here; they are picked up on-demand at user-interaction
// moments (terminal focus-gain, status-panel open, pre-batch-op refresh) from
// the TUI layer.
package watch

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// fsnotifyDebounce coalesces the 3-5 file writes a single git operation
// produces (HEAD + index + refs/heads/<branch> + maybe packed-refs).
const fsnotifyDebounce = 500 * time.Millisecond

// pollingInterval is how often the polling watcher checks for git file changes.
const pollingInterval = 2 * time.Second

// trackedGitFiles is the canonical list of .git/ basenames that indicate a
// meaningful repository state change. Both the fsnotify and polling watchers
// use this list.
var trackedGitFiles = []string{
	"HEAD", "index", "FETCH_HEAD", "ORIG_HEAD", "MERGE_HEAD", "packed-refs", "config",
}

// trackedGitFilesSet is the map form of trackedGitFiles for O(1) lookup in
// the fsnotify event handler.
var trackedGitFilesSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(trackedGitFiles))
	for _, f := range trackedGitFiles {
		m[f] = struct{}{}
	}
	return m
}()

// watcher is the internal contract both the fsnotify and polling implementations
// satisfy. It is intentionally unexported; callers use Service.
type watcher interface {
	register(*git.Repository)
	close() error
}

// Service is the top-level handle for the watch subsystem. Construct one with
// New in the application entry point, register repos as they are loaded, and
// Close on shutdown.
type Service struct {
	w watcher
}

// New constructs a Service. It uses fsnotify on native systems and falls back
// to a polling-based watcher in container environments or when fsnotify is
// unavailable. New always succeeds.
func New() *Service {
	return &Service{w: newWatcher()}
}

// newWatcher picks the appropriate watcher implementation for the current
// runtime environment.
func newWatcher() watcher {
	if isContainerEnvironment() {
		log.Printf("watch: container environment detected, using polling watcher")
		return newPollingWatcher()
	}
	fw, err := newFSWatcher()
	if err != nil {
		log.Printf("watch: fsnotify unavailable (%v), falling back to polling", err)
		return newPollingWatcher()
	}
	return fw
}

// Register starts watching the given repository. Safe to call from any
// goroutine. Registering the same repo twice is a no-op.
//
// Repos may be registered before they finish loading (Branch may be nil at
// hook time). The watcher tolerates that and will start reacting once the
// first real event arrives.
func (s *Service) Register(r *git.Repository) {
	if s == nil || r == nil {
		return
	}
	s.w.register(r)
}

// Close shuts the watcher down. Safe to call multiple times.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	return s.w.close()
}

// isContainerEnvironment reports whether the current process is running inside
// a container (Docker, Podman, LXC, Kubernetes, etc.), where inotify-based
// file watching is often unreliable due to volume-mount semantics.
func isContainerEnvironment() bool {
	// Docker places /.dockerenv in the container root filesystem.
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Podman places /run/.containerenv in the container.
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	// OCI runtimes, Podman, and systemd-nspawn set $container.
	if os.Getenv("container") != "" {
		return true
	}
	// Kubernetes injects KUBERNETES_SERVICE_HOST into every pod.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	// Check cgroup paths for known container hierarchy names.
	// /proc/1/cgroup is the init process; /proc/self/cgroup is the fallback
	// when /proc/1 is inaccessible due to PID namespace isolation.
	for _, path := range []string{"/proc/1/cgroup", "/proc/self/cgroup"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		return strings.Contains(content, "docker") ||
			strings.Contains(content, "/lxc/") ||
			strings.Contains(content, "/kubepods")
	}
	return false
}
