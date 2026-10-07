<p align="center">
  <img width="225" height="115" alt="image" src="https://github.com/user-attachments/assets/a29b5502-f081-4b06-ae8e-0ef97fbf9028" />
</p>

<p align="center">
  A TUI for GitHub pull requests.
</p>


<img width="3840" height="2160" alt="readme-dashboard" src="https://github.com/user-attachments/assets/5fdade54-6e16-4c7f-9819-1e5bc8884fdc" />



## Features

- **Dashboard** - Auto-discovers repos, lists PRs across *My PRs*, *Needs Review*, *Involving*, *All* (open PRs, loaded as you scroll), and *Recent* tabs with a live preview pane.
- **Jump to repo/PR** - `Ctrl+P` to fuzzy-find and jump to any PR.
- **Open a PR directly** - `pho pr 123` to launch straight into a PR's detail view.
- **PR detail** - Browse description, diff, comments, and commits. Sidebar for files and CI checks.
- **Diff navigation** - Line-by-line cursor, `gg`/`G`, `Ctrl+d`/`Ctrl+u`, visual mode for selecting ranges.
- **Inline reviews** - Draft inline comments on diff lines, edit, discard, and batch-submit with a review event. Just like Github web UI.
- **Comments & approvals** - Post top-level comments, review comments, or approve directly.
- **PR actions** - Edit title/body, merge (with method selection), close/reopen, and checkout the branch locally.
- **Commit view** - Inspect individual commits and their diffs.
- **Search** - `/` to search within diffs and descriptions; `n`/`N` to jump between matches.
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

Binary lands in `$(go env GOPATH)/bin/pho`.

Or pin a specific version:

```
go install github.com/utkarsh261/pho/cmd/pho@v0.1.0
```

Add it to the `$PATH`:
```
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
````

## Usage

Right now, pho looks at only the `cwd` and its direct children directories (if they are actually git repos). So if you have some repositories cloned in a directory, you can either open pho in that directory or: 

```
$(go env GOPATH)/bin/pho -root ~/path/to/dir/containing/all/cloned/repositories
```
or simply start it in the current directory

```
pho
```

## Requirements

- Go 1.25+
- Git
- [GitHub CLI (`gh`)](https://cli.github.com) — run `gh auth login` to authenticate

## Build

```
go build -o pho ./cmd/pho
```

With `just`:

```
just build
```

## Run

```
./pho
```

Flags:

| Flag | Description |
|------|-------------|
| `--version` | Print version and exit |
| `--debug` | Enable debug logging |
| `--reset` | Clear all caches and exit |
| `--config <path>` | Path to config file |
| `--root <dir>` | Root directory to scan for git repos (default `.`) |

## Test

```
go test ./...
```

With `just`:

```
just test
```

## Vet

```
go vet ./...
```

With `just`:

```
just vet
```

## Logs

```
tail -f ~/.local/state/pho/debug.log
```

## Why
So that i can build it exactly how i want a tool which i use on a regular basis to be. 
