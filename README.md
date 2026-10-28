<p align="center">
  <img width="225" height="115" alt="image" src="https://github.com/user-attachments/assets/a29b5502-f081-4b06-ae8e-0ef97fbf9028" />
</p>

<p align="center">
  A TUI for GitHub pull requests.
</p>


<img width="3840" height="2160" alt="readme-dashboard" src="https://github.com/user-attachments/assets/5fdade54-6e16-4c7f-9819-1e5bc8884fdc" />



## Features

- **Dashboard** - Auto-discovers repos, lists PRs across *My PRs*, *Needs Review*, *Involving*, *All* (open PRs, loaded as you scroll), and *Recent* tabs with a live preview pane.
- **Jump to repo/PR** - `Ctrl+P` to fuzzy-find and jump to any PR; inside a PR, `Ctrl+P` jumps to any file in its diff.
- **Open a PR directly** - `pho pr 123` to launch straight into a PR's detail view.
- **PR detail** - Browse description, diff, comments, and commits. Sidebar for files and CI checks.
- **Stacked PRs** - PRs built on another open PR's branch, and GitHub's native stacks, are listed together with a rail joining them, and the preview maps the whole stack with each PR's CI and review state. In a PR, `{`/`}` step down/up the stack on the same tab and `S` lists it.
- **CI logs** - `Enter` on a GitHub Actions check shows the end of its job log with error lines highlighted; `n`/`N` jumps between errors and `r` re-runs the job.
- **Diff navigation** - Line-by-line cursor, `gg`/`G`, `Ctrl+d`/`Ctrl+u`, visual mode for selecting ranges.
- **Inline reviews** - Draft inline comments on diff lines, edit, discard, and batch-submit with a review event. Just like Github web UI.
- **Comments & approvals** - Post top-level comments, review comments, or approve directly.
- **PR actions** - Edit title/body, merge (with method selection), close/reopen, and checkout the branch locally.
- **Commit view** - Inspect individual commits and their diffs.
- **Search** - `/` to search within diffs and descriptions; `n`/`N` to jump between matches.
- **Themes** - Catppuccin Mocha, Tokyo Night, Gruvbox, Nord, Dracula and Rosé Pine. Press `T` to preview them live.
- Works with Github Enterprise as well.

## Install

With Homebrew (macOS and Linux):

```
brew install utkarsh261/tap/pho
```

Or with Go:

```
go install github.com/utkarsh261/pho/cmd/pho@latest
```

Binary lands in `$(go env GOPATH)/bin/pho`. To pin a version, swap `@latest` for a tag from the [releases page](https://github.com/utkarsh261/pho/releases), e.g. `@v0.1.49`.

Add it to the `$PATH`:
```
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
```

## Requirements

- Git
- [GitHub CLI (`gh`)](https://cli.github.com), logged in with `gh auth login`. pho reuses its token, including for GitHub Enterprise hosts.
- Go 1.25+, only if you install with `go install` or build from source

## Usage

pho looks at the current directory and its direct children for git repos with a GitHub remote. Start it in a directory holding your clones:

```
pho
```

or point it at one:

```
pho --root ~/path/to/dir/containing/all/cloned/repositories
```

To open a pull request straight in its detail view:

```
pho pr 123
```

Flags work before or after `pr <number>`:

| Flag | Description |
|------|-------------|
| `--root <dir>` | Directory to scan for git repos (default `.`) |
| `--theme <name>` | Theme for this run, overriding the config file |
| `--config <path>` | Config file to use (default `~/.config/pho/config.toml`) |
| `--debug` | Log at debug level (also `PHO_DEBUG=1`) |
| `--reset` | Clear the SQLite and repo discovery caches before starting |
| `--version` | Print the version and exit |

## Configuration

pho reads `$XDG_CONFIG_HOME/pho/config.toml`, which defaults to `~/.config/pho/config.toml`. It runs fine without one. Every key is optional; the values below are the defaults.

```toml
[repos]
pin = []                   # "owner/repo" entries to list first
exclude = []               # "owner/repo" entries to hide

[dashboard]
max_prs = 300              # open PRs the All tab loads at most
group_stacks = true        # list a stack's PRs together, top of the stack first

[palette]
max_prs = 300              # PRs the Ctrl+P jump list holds

[diff]
collapse_lines = 1000      # files with more changed lines start collapsed
collapse_line_width = 5000 # so do files with a line longer than this
max_lines = 5000           # files past this can only be opened on GitHub
max_line_width = 20000     # same, for line length
row_budget = 20000         # past this many diff rows, the largest files collapse

[ui]
theme = "pho-default"      # see Themes below
background = "theme"       # or "terminal"

[cache]
dir = "~/.cache/pho"       # where cache.db lives
max_memory_mb = 16         # in-memory cache size

[logging]
file = "~/.local/state/pho/debug.log"
level = "info"             # debug, info, warn or error
```

## Themes
<img width="1600" height="900" alt="pho-themes" src="https://github.com/user-attachments/assets/7d452bdc-015c-43b3-bcaf-cd24e83f562c" />


Press `T` anywhere to open the theme picker. Arrowing through it repaints the screen with each theme; `Enter` keeps one and saves it to your config, `Esc` puts the old one back. You can also set it in the config:

```toml
[ui]
theme = "catppuccin-mocha"  # pho-default (the default), catppuccin-mocha, tokyo-night,
                            # gruvbox-dark, nord, dracula, rose-pine
background = "theme"        # or "terminal" to keep your terminal's own background
```

or for one run with `pho --theme nord`.

Themes paint their own background so text stays readable whatever your terminal is set to. With `background = "terminal"` (handy for transparent terminals) contrast is only guaranteed when your terminal background matches the theme's. Colours are checked for contrast on truecolor and 256-colour terminals; on 16-colour terminals the cursor row switches to reverse video.

## Logs

```
tail -f ~/.local/state/pho/debug.log
```

## Development

```
just build      # go build -o pho ./cmd/pho
just test       # go test ./...
just test-race  # go test -race -count=1 ./...
just vet        # go vet ./...
```

## Why
So that i can build it exactly how i want a tool which i use on a regular basis to be. 
