# Custom commands

Bind a key to your own shell command in pho's config (`~/.config/pho/config.toml`). On the dashboard the key runs the command for the selected PR; in a PR's detail view, for that PR.

```toml
[[keybindings]]
key = "w"
name = "worktree"   # shown in the ? keymap; the command is shown if left out
command = "git fetch origin pull/{{.PrNumber}}/head && git worktree add ../wt/pr-{{.PrNumber}} FETCH_HEAD"

[[keybindings]]
key = "i"
command = "lazygit"

[[keybindings]]
key = "P"
name = "tmux window"
command = "tmux new-window -c {{.RepoPath}} -n pr-{{.PrNumber}}"
```

## Fields

`{{.Name}}` fills in a value from the PR:

| Field | Example | Notes |
|-------|---------|-------|
| `RepoName` | `owner/repo` | |
| `RepoPath` | `/home/me/src/repo` | The local clone. A command that uses it doesn't run for a repo pho found no clone of. |
| `PrNumber` | `123` | |
| `HeadRefName` | `feature/login` | The PR's branch |
| `BaseRefName` | `main` | The branch it merges into |
| `HeadSHA` | `9bb7485…` | The head commit |
| `Author` | `octocat` | |
| `URL` | `https://github.com/owner/repo/pull/123` | |

## Quoting

Values arrive already quoted for the shell, so a branch named `x; rm -rf ~` or `$(curl …)` stays one harmless word. Branch names and authors come from whoever opened the PR, so this matters.

- Don't put quotes around a field: `"{{.HeadRefName}}"` or `'{{.HeadRefName}}'`. Inside your own quotes pho's quoting can't protect the value, so pho refuses such a command at startup. `{{.PrNumber}}` is only digits and may sit inside quotes.
- A value can still start with `-`, and quoting doesn't stop a program reading it as an option. Use `pull/{{.PrNumber}}/head` or `{{.HeadSHA}}` to refer to the PR in git, or put `--end-of-options` (git) or `--` before branch names.
- For git, `pull/{{.PrNumber}}/head` is also the ref that works for PRs from forks; `origin {{.HeadRefName}}` only finds branches pushed to your own remote.

## Template functions

Commands are Go templates. Functions see the raw value, and only what ends up printed is quoted, so these work as you'd expect:

```
git show {{slice .HeadSHA 0 7}}
{{if ne .BaseRefName "main"}}echo stacked on {{.BaseRefName}}{{end}}
```

The functions allowed are `and or not eq ne lt le gt ge len slice`. Others such as `printf`, `html` or `urlquery` re-print an already quoted value and could break its quoting, so they're refused at startup, as are `define` and `template`.

## Keys

A key is one character that pho doesn't already use. Free letters today: `f i p u w A B F I P Q V X Y`, the digits `0 5-9`, and most punctuation. A clash is refused at startup with the list of free keys.

Keys typed into the palette, search, a comment, visual mode or a prompt stay text and never run a command. In a PR opened with `pho pr N`, keys wait until the PR has loaded.

The `?` keymap lists your keys first, under Custom.

## How commands run

- With `sh -c`, in the repo's clone (or pho's directory if pho found no clone).
- The command gets the terminal until it exits, so interactive tools such as `lazygit` or `nvim` work. pho redraws when it's done, which clears anything printed just before exit; end with `; read` to keep output on screen.
- A non-zero exit shows in the status bar, e.g. `Custom command w failed (exit 3)`.

## Errors

pho checks every binding at startup and stops with a message naming the key and the problem: a built-in or repeated key, a key longer than one character, an empty command, an unknown field such as `{{.PRNumber}}`, a field inside quotes, or an unsupported function.

## Coming from gh-dash

The fields match gh-dash's PR fields, plus `HeadSHA` and `URL`. `RepoPath` needs no `repoPaths` map, since pho already knows where each repo is cloned. Drop any quotes around fields when you copy a command over.
