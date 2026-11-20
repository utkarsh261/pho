#!/usr/bin/env bash
# End-to-end check of [[keybindings]] custom commands in the real pho binary,
# against real GitHub, driven through tmux by the run-pho driver.
#
#   scripts/e2e/custom-keys.sh [owner/repo]
#
# Needs gh logged in, tmux and go. The repo (default
# utkarsh261/pho-large-diff-scratch) must have open PRs; nothing on GitHub is
# changed. Each step's screen is saved under $PHO_RUN_DIR/screens.
set -euo pipefail

REPO="${1:-utkarsh261/pho-large-diff-scratch}"
TMP="${TMPDIR:-/tmp}"
export PHO_RUN_DIR="${PHO_RUN_DIR:-${TMP%/}/pho-custom-keys-e2e}"
D=.claude/skills/run-pho/driver.sh
S=ck # tmux session
OUT="$PHO_RUN_DIR/out"
SCREENS="$PHO_RUN_DIR/screens"
step=0

fail() {
	echo "FAIL: $*" >&2
	tmux capture-pane -t "$S" -p >&2 2>/dev/null || true
	exit 1
}
pass() { echo "ok: $*"; }
snap() {
	step=$((step + 1))
	tmux capture-pane -t "$S" -p >"$SCREENS/$(printf %02d "$step")-$1.txt"
}
screen() { tmux capture-pane -t "$S" -p; }
on_screen() { screen | grep -qF -- "$1"; }
wait_file() {
	for _ in $(seq 1 100); do
		[ -s "$1" ] && return 0
		sleep 0.1
	done
	fail "$1 never written"
}
no_file() {
	sleep 1
	[ ! -e "$1" ] || fail "$1 exists, so the command ran: $(cat "$1")"
}
config() { # config <name> <toml>: write a config file, print its path
	printf '%s\n' "$2" >"$PHO_RUN_DIR/$1.toml"
	echo "$PHO_RUN_DIR/$1.toml"
}

rm -rf "$PHO_RUN_DIR"
mkdir -p "$OUT" "$SCREENS"
trap '$D stop $S >/dev/null 2>&1 || true' EXIT

$D build >/dev/null
$D repo dash "$REPO" >/dev/null
CLONE="$PHO_RUN_DIR/repos/clone"
gh repo clone "$REPO" "$CLONE" -- -q --filter=blob:none

FIELDS='{ printf "%s\n" {{.RepoName}} {{.RepoPath}} {{.PrNumber}} {{.HeadRefName}} {{.BaseRefName}} {{.HeadSHA}} {{.Author}} {{.URL}}; pwd; } > '"$OUT"'/fields.tmp && mv '"$OUT"'/fields.tmp '"$OUT"'/fields'
GOOD=$(config good "
[[keybindings]]
key = \"w\"
name = \"dump fields\"
command = '''$FIELDS'''

[[keybindings]]
key = \"i\"
name = \"ask\"
command = \"echo CUSTOM-RUN; read x; echo got-\$x > $OUT/interactive\"

[[keybindings]]
key = \"f\"
command = \"exit 3\"

[[keybindings]]
key = \"u\"
name = \"worktree\"
command = \"git fetch -q origin {{.HeadRefName}} && git worktree add -q ../wt-{{.PrNumber}} FETCH_HEAD && echo done > $OUT/worktree\"
")

# Field values must match what GitHub says about that PR.
check_fields() { # check_fields <repo dir>
	wait_file "$OUT/fields"
	local f=() line
	while IFS= read -r line; do f+=("$line"); done <"$OUT/fields"
	[ "${#f[@]}" -eq 9 ] || fail "fields has ${#f[@]} lines"
	local want
	want=$(gh pr view "${f[2]}" -R "$REPO" --json headRefName,baseRefName,headRefOid,author,url \
		--jq '[.headRefName, .baseRefName, .headRefOid, .author.login, .url] | join("\n")')
	[ "${f[0]}" = "$REPO" ] || fail "RepoName ${f[0]}"
	[ "${f[1]}" = "$1" ] || fail "RepoPath ${f[1]}, want $1"
	[ "$(printf '%s\n' "${f[3]}" "${f[4]}" "${f[5]}" "${f[6]}" "${f[7]}")" = "$want" ] ||
		fail "fields for #${f[2]}: $(printf '%s|' "${f[@]:3:5}") want $(echo "$want" | tr '\n' '|')"
	[ "$(cd "${f[8]}" && pwd -P)" = "$(cd "$1" && pwd -P)" ] || fail "ran in ${f[8]}, want $1"
	rm "$OUT/fields"
}

# 1. Bad configs stop pho at startup with a message naming the problem.
bad() { # bad <name> <toml> <expected text>
	$D start $S dash --config "$(config "$1" "$2")" >/dev/null
	$D wait $S "[pho exited]" 30 >/dev/null
	# The error is one long line that wraps at the pane's width.
	screen | tr -d '\n' | grep -qF -- "$3" || fail "bad config $1: no \"$3\""
	snap "bad-$1"
	pass "bad config $1 rejected"
}
bad reserved '[[keybindings]]
key = "e"
command = "true"' 'key is a built-in pho key; free keys: f i p u w'
bad duplicate '[[keybindings]]
key = "w"
command = "true"
[[keybindings]]
key = "w"
command = "true"' 'is defined more than once'
bad modifier '[[keybindings]]
key = "ctrl+x"
command = "true"' 'key must be a single character'
bad field '[[keybindings]]
key = "w"
command = "echo {{.PRNumber}}"' 'PRNumber'

# 2. Dashboard: the selected PR's fields, run from the repo dir.
$D start $S dash --config "$GOOD" >/dev/null
$D wait $S "#" 60 >/dev/null
sleep 2
$D keys $S w
check_fields "$PHO_RUN_DIR/repos/dash"
$D wait $S "?" 10 >/dev/null
snap dashboard-after-w
pass "dashboard: fields match GitHub, ran in the repo"
$D keys $S j w
check_fields "$PHO_RUN_DIR/repos/dash"
pass "dashboard: second PR after j"

# 3. The command owns the terminal: it prompts, reads, and pho comes back.
$D keys $S i
$D wait $S CUSTOM-RUN 10 >/dev/null
snap interactive-prompt
on_screen "Keymap" && fail "pho still drawn while the command runs"
$D type $S hello
$D keys $S Enter
wait_file "$OUT/interactive"
[ "$(cat "$OUT/interactive")" = "got-hello" ] || fail "command read $(cat "$OUT/interactive")"
$D wait $S "Keymap" 10 >/dev/null
snap interactive-back
pass "interactive: command read input, pho redrew"

# 4. A failing command leaves pho running with a notice.
$D keys $S f
$D wait $S "Custom command f failed (exit 3)" 10 >/dev/null
snap failure
$D keys $S j
sleep 0.5
on_screen "failed (exit 3)" && fail "notice outlived the next key"
pass "failure: notice shown, cleared by the next key"

# 5. Palette: the key is text.
$D keys $S C-p
$D keys $S w
no_file "$OUT/fields"
snap palette
$D keys $S Escape
pass "palette: w typed, nothing ran"

# 6. Overlay lists the custom keys first.
$D keys $S "?"
$D wait $S "dump fields" 10 >/dev/null
on_screen "worktree" || fail "overlay misses worktree"
snap overlay-dashboard
$D keys $S Escape
pass "overlay: custom keys listed on the dashboard"
$D stop $S

# 7. PR detail in a real clone: the issue's worktree case.
PR=$(gh pr list -R "$REPO" --state open --json number --jq '.[-1].number')
$D start $S clone --config "$GOOD" pr "$PR" >/dev/null
$D wait $S "Files " 60 >/dev/null
$D settle >/dev/null
$D keys $S w
check_fields "$CLONE"
pass "PR detail #$PR: fields match GitHub"
$D keys $S u
wait_file "$OUT/worktree"
git -C "$CLONE" worktree list | grep -q "wt-$PR" || fail "no wt-$PR worktree"
[ -n "$(ls -A "$PHO_RUN_DIR/repos/wt-$PR")" ] || fail "worktree is empty"
[ "$(git -C "$PHO_RUN_DIR/repos/wt-$PR" rev-parse HEAD)" = "$(gh pr view "$PR" -R "$REPO" --json headRefOid --jq .headRefOid)" ] ||
	fail "worktree not at the PR head"
$D wait $S "Files " 10 >/dev/null
snap worktree-back
pass "PR detail: worktree created at the PR head, pho redrew"

# 8. PR detail input modes take the key as their own.
$D keys $S 2
$D wait $S "@@" 30 >/dev/null
$D keys $S / w
$D wait $S "/ w" 5 >/dev/null
no_file "$OUT/fields"
snap search
$D keys $S Escape Escape 1
$D keys $S C
$D wait $S "New comment" 5 >/dev/null
$D keys $S w
no_file "$OUT/fields"
snap compose
$D keys $S Escape
$D keys $S C-p
$D keys $S w
no_file "$OUT/fields"
snap file-picker
$D keys $S Escape
pass "PR detail: search, compose and file picker take w as text"

$D keys $S "?"
$D wait $S "dump fields" 10 >/dev/null
snap overlay-pr-detail
$D keys $S Escape
pass "overlay: custom keys listed in PR detail"
$D stop $S

# 9. No keybindings: nothing new shows up, built-in keys still work.
$D start $S clone --config "$(config none '[ui]
theme = "pho-default"')" pr "$PR" >/dev/null
$D wait $S "Files " 60 >/dev/null
$D settle >/dev/null
$D keys $S "?"
sleep 1
on_screen "Custom" && fail "Custom group without bindings"
snap overlay-none
$D keys $S Escape E
$D wait $S "Edit: [t]itle or [b]ody?" 5 >/dev/null
$D keys $S Escape
pass "no keybindings: overlay unchanged, E still prompts"

git -C "$CLONE" worktree remove --force "../wt-$PR"
echo "all custom key checks passed; screens in $SCREENS"
