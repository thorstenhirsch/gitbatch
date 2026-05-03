[![MIT License](https://img.shields.io/badge/license-MIT-brightgreen.svg)](/LICENSE) [![Go Report Card](https://goreportcard.com/badge/github.com/thorstenhirsch/gitbatch)](https://goreportcard.com/report/github.com/thorstenhirsch/gitbatch)

## gitbatch

Managing multiple git repositories is easier than ever. I (*was*) often end up working on many directories and manually pulling updates etc. To make this routine faster, I created a simple tool to handle this job. Although the focus is batch jobs, you can still do de facto micro management of your git repositories (e.g *add/reset, stash, commit etc.*). And for the more complex stuff, you can always open lazygit from within gitbatch.

Note: This is my AI playing field, so expect weird code.

![gitbatch demo](.github/assets/gitbatch-demo.gif)

## Installation

Download the latest release artifact from the [GitHub Releases page](https://github.com/thorstenhirsch/gitbatch/releases/latest), then extract and place the binary on your `PATH`.

Typical release artifacts include binaries for `darwin`, `linux`, and `windows`.

Example (macOS/Linux):
```bash
# 1) Download the archive for your OS/architecture from the latest release page
# 2) Extract it
tar -xzf gitbatch_<version>_<os>_<arch>.tar.gz

# 3) Move binary to PATH
chmod +x gitbatch
sudo mv gitbatch /usr/local/bin/gitbatch
```

Windows:
1. Download the `windows` release artifact from [Releases](https://github.com/thorstenhirsch/gitbatch/releases/latest).
2. Extract `gitbatch.exe`.
3. Add its directory to your `PATH`.

## Use

Run `gitbatch` from the parent directory of your git repositories. The TUI starts in **pull** mode.

```bash
gitbatch                          # scan current directory
gitbatch -d ~/src                 # scan a specific directory
gitbatch -d ~/src -r 2            # scan recursively (depth 2)
gitbatch -q                       # quick mode: batch pull without TUI
gitbatch -q -m merge              # quick mode: batch merge
gitbatch -m push                  # start TUI in push mode
gitbatch --help                   # show all options
```

### Key bindings

| Key | Action |
|-----|--------|
| `↑`/`k`, `↓`/`j` | Navigate repositories |
| `g`/`Home`, `G`/`End` | Jump to top / bottom |
| `PgUp`/`Ctrl+B`, `PgDn`/`Ctrl+F` | Page up / down |
| `Ctrl+U`, `Ctrl+D` | Half-page up / down |
| `←`/`h`, `→`/`l` | Scroll commit message |
| `Space` | Toggle queue (tag/untag for batch) |
| `Enter` | Start queued jobs |
| `a` / `A` | Tag all / untag all |
| `m` | Cycle operation mode (pull → merge → rebase → push) |
| `W` | Toggle worktree mode |
| `Tab` | Open lazygit for selected repo |
| `f` | Fetch selected repo |
| `p` | Pull selected repo |
| `P` | Push selected repo |
| `n` | Create branch, or create worktree in worktree mode |
| `d` | Delete selected linked worktree in worktree mode |
| `L` | Lock/unlock selected linked worktree in worktree mode |
| `X` | Prune stale worktrees in worktree mode |
| `c` | Commit (or clear error message) |
| `S` | Stash local changes |
| `O` / `D` | Pop / drop stash |
| `b` | Show branches panel |
| `B` | Expand/collapse all branches in table |
| `r` | Show remotes panel |
| `s` | Show status panel |
| `R` | Force refresh all repositories |
| `t` | Toggle sorting by name / last modified time |
| `?` | Toggle help |
| `q` / `Ctrl+C` | Quit |

Inside the **branches** and **remotes** panels: `Space`/`c` to checkout, `d` to delete.

### Worktree mode

Press `W` to switch the overview into **worktree mode**. Repositories that share a common Git directory are grouped into a single worktree family so you can inspect the main worktree and linked worktrees together.

Available worktree actions:

- `n` — create a new linked worktree by entering a branch name and path
- `d` — remove the selected linked worktree
- `L` — lock or unlock the selected linked worktree
- `X` — prune stale worktree metadata

When you type a branch name in the worktree prompt, `gitbatch` now prefills the path using the same default convention as `wt`: a sibling directory named `<repo>.<branch-sanitized>`, for example `myproject.feature-auth`.

Worktree rows show `[main]` for the primary worktree, a shortened name for linked worktrees, and markers such as `[L]` for locked or `[P]` for prunable entries.

The status panel (`s`) also reflects the selected worktree, including whether it is primary or linked and any lock/prune reasons reported by Git.

### Configuration

Configuration is stored at `$XDG_CONFIG_HOME/gitbatch/config.yml` (macOS: `~/Library/Application Support/gitbatch/config.yml`).

```yaml
mode: pull          # default mode: fetch | pull | merge | rebase | push
recursion: 1        # directory scan depth
quick: false        # start in quick mode by default
```

## Credits
- [go-git](https://github.com/go-git/go-git) for git interface (partially)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for terminal user interface
- [Lipgloss](https://github.com/charmbracelet/lipgloss) for terminal styling
- [viper](https://github.com/spf13/viper) for configuration management
- [kingpin](https://github.com/alecthomas/kingpin) for command-line flag&options
