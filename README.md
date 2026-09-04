<p align="center">
  <img width="225" height="115" alt="image" src="https://github.com/user-attachments/assets/a29b5502-f081-4b06-ae8e-0ef97fbf9028" />
</p>

<p align="center">
  A TUI for GitHub pull requests.
</p>

<img width="1468" height="889" alt="image" src="https://github.com/user-attachments/assets/f83b174f-afd9-4602-bc98-7355b698cb71" />


## Features

- **Dashboard** - Auto-discovers repos, lists PRs across *My PRs*, *Needs Review*, *Involving*, and *Recent* tabs with a live preview pane.
- **Jump to repo/PR** - `Ctrl+P` to fuzzy-find and jump to any PR.
- **PR detail** - Browse description, diff, comments, and commits. Sidebar for files and CI checks.
- **Diff navigation** - Line-by-line cursor, `gg`/`G`, `Ctrl+d`/`Ctrl+u`, visual mode for selecting ranges.
- **Inline reviews** - Draft inline comments on diff lines, edit, discard, and batch-submit with a review event. Just like Github web UI.
- **Comments & approvals** - Post top-level comments, review comments, or approve directly.
- **PR actions** - Edit title/body, merge (with method selection), close/reopen, and checkout the branch locally.
- **Commit view** - Inspect individual commits and their diffs.
- **Search** - `/` to search within diffs and descriptions; `n`/`N` to jump between matches.

## Install

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

### Open a PR directly

Jump straight into a PR's detail view, skipping the dashboard:

```
pho pr 123
```

The PR number resolves against the repos pho discovers at startup (the working directory and its direct children):

- run from a repo's root → that repo is used;
- a directory containing exactly one repo → that repo is used;
- several repos → a picker lists them; pick one and the PR opens in it (esc cancels);
- no repos → pho starts on the dashboard and shows an error.

If the PR doesn't exist (or fails to load), the detail view shows an error panel — `r` retries, `esc` goes back to the dashboard.

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
