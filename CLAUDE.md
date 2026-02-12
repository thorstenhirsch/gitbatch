# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build ./cmd/gitbatch/main.go        # build
go run ./cmd/gitbatch/main.go           # run from current directory
go run ./cmd/gitbatch/main.go -d ~/src  # run on specific directory
go run ./cmd/gitbatch/main.go -q        # quick mode (no TUI, batch pull)
```

Cross-platform release builds use goreleaser with CGO_ENABLED=0. Version is injected via ldflags: `-X main.version=`.

## Testing

```bash
go test ./...                           # all tests
go test ./internal/command/...          # single package
go test -run TestFetchName ./internal/command/...  # single test
go test -v -count=1 ./...              # verbose, no cache
```

Tests use `testify` for assertions. Many command tests require a real git repo created via helpers in `internal/gittest/`.

## Architecture

**gitbatch** is a batch git operations manager with a Bubbletea TUI. It discovers git repos in a directory tree and runs fetch/pull/merge/rebase/push across them.

### Package structure

- **`cmd/gitbatch/`** — Entry point. Parses CLI flags (kingpin), creates `app.App`, calls `app.Run()`.
- **`internal/app/`** — App orchestration. Config loading (viper, OS-specific paths), directory discovery, quick mode execution.
- **`internal/git/`** — Core `Repository` type wrapping go-git. Event-driven pub/sub system with async event queues (git, state, log). Semaphore-based concurrency limiting (one slot per CPU core).
- **`internal/command/`** — Git command execution. Runs git via `exec.Command` with timeout/context support. Credential prompt detection (kills process on password prompt). Schedules work through `ScheduleGitCommand` → git event queue → state evaluation pipeline.
- **`internal/tui/`** — Bubbletea Model with two views: Overview (repo table) and Focus (single repo detail). Side panels for branches, remotes, commits, stashes. Lipgloss styling.
- **`internal/job/`** — Job abstraction mapping high-level operations (FetchJob, PullJob, etc.) to command execution.
- **`internal/load/`** — Parallel repo initialization using worker pool pattern.
- **`internal/errors/`** — Custom error types for git operations and credential detection.

### Key patterns

**Event-driven concurrency:** Repositories use a pub/sub event system. Git commands, state evaluation, and trace logging each have their own async event queue. The git queue uses a global weighted semaphore (`runtime.GOMAXPROCS(0)` slots) to limit concurrent git operations.

**Repository lifecycle:** `FastInitializeRepo` creates a minimal repo struct (used during loading). `InitializeRepo` adds full component loading (branches, remotes, stashes). Event queues start goroutines immediately on creation.

**Command execution pipeline:** `ScheduleGitCommand` → git queue → `AttachGitCommandWorker` listener executes with timeout → `ScheduleStateEvaluation` forwards result to state queue → state listener updates repo status and triggers UI refresh.

**Hook registration:** Packages register `RepositoryHook` functions via `init()` (e.g., `command` package auto-attaches its git worker to every new repository).

**TUI update loop:** Bubbletea model receives `RepositoryUpdateMsg` via a channel. Updates are throttled to ~60 FPS. The model polls for job completion status.
