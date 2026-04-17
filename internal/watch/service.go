// Package watch detects external git operations via fsnotify watches on each
// repository's .git/ directory. When a watched file under .git/ changes
// (HEAD, index, refs), the repository is routed through
// command.RequestExternalRefresh, reusing the existing refresh pipeline.
//
// Working-tree edits (unstaged changes to tracked files, new untracked files)
// are not detected here; they are picked up on-demand at user-interaction
// moments (terminal focus-gain, status-panel open, pre-batch-op refresh) from
// the TUI layer.
package watch

import (
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// Service is the top-level handle for the watch subsystem. Construct one in
// the application entry point, register repos as they're loaded, and Close
// on shutdown.
type Service struct {
	fs *fsWatcher
}

// New constructs a Service. The fsnotify watcher is created eagerly so an
// error surfaces at startup.
func New() (*Service, error) {
	fs, err := newFSWatcher()
	if err != nil {
		return nil, err
	}
	return &Service{fs: fs}, nil
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
	s.fs.register(r)
}

// Close shuts the watcher down. Safe to call multiple times.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	return s.fs.close()
}
