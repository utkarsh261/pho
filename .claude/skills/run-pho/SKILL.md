---
name: run-pho
description: Build, run and drive the pho TUI against real GitHub PRs. Use when asked to run pho, start it, try a change in the real app, open a PR in pho, screenshot or read its screen, press keys in it, or check what a key does (diff navigation, collapsing, comments, opening in browser).
---

pho is a Bubble Tea terminal UI. An agent drives it through
`.claude/skills/run-pho/driver.sh`, which runs pho inside a detached tmux
session, sends keys one at a time, and reads the screen back as text. All
paths are relative to the repo root (or a worktree root); the driver builds
`./cmd/pho` from the current directory, so run it from the checkout you want
to test.

## Prerequisites

- Go (version from `go.mod`), `tmux` on `PATH`, and `gh` logged in
  (`gh auth status` shows a host). pho reads the token from `gh`.
- Verified on macOS with Homebrew tmux.

## Run (agent path)

```bash
D=.claude/skills/run-pho/driver.sh
$D build                                  # builds into $PHO_RUN_DIR (default ${TMPDIR:-/tmp}/pho-run)
$D repo pho utkarsh261/pho                # a dir whose git origin is the repo; no clone needed
$D start p pho pr 30                      # session "p": runs `pho --debug pr 30` in that dir
$D wait p "Files " 60                     # the file list has loaded
$D settle                                 # and pho has finished (re)loading the diff
$D keys p 2                               # Diff tab
$D wait p "@@" 30
$D screen p                               # the whole screen as text (this is the "screenshot")
$D keys p j j
$D cursor p                               # diff line under the cursor
$D status p                               # bottom hint/status bar
$D keys p o; $D opened                    # URLs pho tried to open (no browser is launched)
$D log '"pr load diff"'                   # grep pho's debug log (timings, errors)
$D stop p
```

Higher-level helpers (verified on a PR's Diff tab):

```bash
$D jumpfile s 1                 # focus file list, select file #1, Enter
$D goto s "return 500"          # press j until the cursor line contains the text
$D draft s 0 "local only"       # Space, j*0, c, type body, Enter -> saves a draft
$D keys s D y                   # discard all drafts
```

Pressing `v`, typing a body and Enter **submits a real review to GitHub** with
every draft. Only do that on a throwaway PR.

Keys worth knowing in PR detail: `1-4` tabs, `Tab`/`h`/`l` move focus between
the file list and the content pane, `j/k` move, `gg`/`G` top/bottom, `Space`
visual mode, `c` comment, `v` review, `o` open PR in browser, `/` search,
`R` refresh, `?` keymap, `q`/`Esc` back.

## Run (human path)

```bash
just build && ./pho pr 30
```

Run it inside a clone of the repo; it takes over the terminal.

## Test

```bash
go vet ./... && go test -race -count=1 ./...
```

## Gotchas

- **Send keys one at a time.** tmux delivers a burst like `kkkk` in one read,
  and Bubble Tea turns it into a single multi-character key that matches no
  binding, so it silently does nothing. `driver.sh keys` sends each key and
  sleeps `PHO_KEY_DELAY` (0.25s) between them. `type` is the exception: use it
  for composer text.
- **Pick a ready marker that exists.** `Checks` only appears when the PR has CI
  checks; `Files <n>` (the file list header) always appears once the diff has
  loaded. On the Diff tab, wait for `@@`.
- **Don't redirect `XDG_CONFIG_HOME`.** `gh` keeps its login there; pho then
  exits with "no authenticated GitHub hosts found". The driver only redirects
  `XDG_CACHE_HOME` and `XDG_STATE_HOME`, so your real cache is untouched and
  the debug log is at `$PHO_RUN_DIR/state/pho/debug.log`.
- **pho finds the repo from the working directory's git `origin`.** `driver.sh
  repo` makes an empty dir with just that remote; that is enough for `pho pr N`.
- **`cursor` is empty when the file list has focus.** The `▎` cursor marker is
  only drawn while the content pane is focused. `h` moves focus to the file
  list; `l` or `Enter` there jumps back into the diff. The screen layout puts
  the content pane at column 44 for the 160-column session the driver uses.
- **Drafts persist** in the run dir's cache between sessions (per PR and head
  commit). Discard them with `D y` before reusing a PR.
- **`pho pr N` loads the diff twice** (once before the head commit is known),
  and the second load resets the diff cursor. Keys sent in between act on a
  diff that is about to be replaced; run `$D settle` after the first `wait`.
  `start` clears the debug log, so `settle` and `log` only see this run.
- **Huge repos can 502 the dashboard query.** pho then prints GitHub's HTML
  error page into the status bar, which hides the key hints. Use a smaller
  repo when you need to read `status`.

## Troubleshooting

- `driver: pho exited while waiting for: ...` - the last lines of the session
  (pho's stderr) are printed above; usually auth (`gh auth login`) or a repo
  that pho can't see.
- `driver: no binary at ...` - run `$D build` from the checkout first.
- `driver: timed out ... waiting for: Checks` - the PR has no CI; wait for
  `Files ` instead.
