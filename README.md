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
run the `gitbatch` command from the parent of your git repositories. For start-up options simply `gitbatch --help`

For more information see the [wiki pages](https://github.com/thorstenhirsch/gitbatch/wiki)

## Credits
- [go-git](https://github.com/src-d/go-git) for git interface (partially)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for terminal user interface
- [Lipgloss](https://github.com/charmbracelet/lipgloss) for terminal styling
- [viper](https://github.com/spf13/viper) for configuration management
- [kingpin](https://github.com/alecthomas/kingpin) for command-line flag&options

