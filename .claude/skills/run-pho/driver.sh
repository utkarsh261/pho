#!/usr/bin/env bash
# Drive pho (a Bubble Tea TUI) inside tmux so an agent can send keys and read
# the screen. Every subcommand is safe to call repeatedly.
#
#   driver.sh build                         build ./cmd/pho into the run dir
#   driver.sh repo <name> <owner/repo>      make a dir whose origin is owner/repo
#   driver.sh start <session> <name> [args] launch pho in that dir (e.g. pr 30)
#   driver.sh wait <session> <text> [secs]  block until text is on screen
#   driver.sh settle [secs]                 block until pho stops (re)loading diffs
#   driver.sh keys <session> <key>...       send keys one at a time
#   driver.sh type <session> <text>         type literal text (composer)
#   driver.sh screen <session>              print the screen (plain text)
#   driver.sh cursor <session>              print the diff line under the cursor
#   driver.sh status <session>              print the status/hint bar
#   driver.sh jumpfile <session> <index>    open file #index from the file list
#   driver.sh goto <session> <text>         press j until the cursor line has text
#   driver.sh draft <session> <extra> <body> Space, j*extra, c, type body, Enter
#   driver.sh opened                        URLs pho tried to open in a browser
#   driver.sh log [pattern]                 grep pho's debug log
#   driver.sh stop <session>                kill the session
#
# Run from the repo root (or a worktree root). State lives in $PHO_RUN_DIR
# (default ${TMPDIR:-/tmp}/pho-run): the binary, cache, debug log, repo dirs
# and the log of browser opens. Your real ~/.cache/pho is never touched.
set -euo pipefail

RUN_DIR="${PHO_RUN_DIR:-${TMPDIR:-/tmp}/pho-run}"
RUN_DIR="${RUN_DIR%/}"
BIN="$RUN_DIR/pho"
KEY_DELAY="${PHO_KEY_DELAY:-0.25}"

die() { echo "driver: $*" >&2; exit 1; }

# A fake `open`/`xdg-open` first on PATH records URLs instead of opening a
# browser, so `o`/`O` can be checked without stealing focus.
fakebin() {
	mkdir -p "$RUN_DIR/fakebin"
	for b in open xdg-open; do
		printf '#!/bin/sh\necho "$@" >> "%s/opened.txt"\n' "$RUN_DIR" >"$RUN_DIR/fakebin/$b"
		chmod +x "$RUN_DIR/fakebin/$b"
	done
}

cursorline() { tmux capture-pane -t "$1" -p | { grep "▎" || true; } | head -1 | cut -c44- | sed 's/[│ ]*$//'; }

diffloads() { { grep -c '"msg":"pr load diff"\|"msg":"pr load commit diff"' "$RUN_DIR/state/pho/debug.log" 2>/dev/null || true; } | head -1; }

key() { tmux send-keys -t "$1" "$2"; sleep "$KEY_DELAY"; }

cmd="${1:-}"
shift || true
case "$cmd" in
build)
	mkdir -p "$RUN_DIR"
	go build -o "$BIN" ./cmd/pho
	echo "$BIN"
	;;
repo)
	[ $# -eq 2 ] || die "usage: repo <name> <owner/repo>"
	d="$RUN_DIR/repos/$1"
	mkdir -p "$d"
	git -C "$d" init -q 2>/dev/null || true
	git -C "$d" remote remove origin 2>/dev/null || true
	git -C "$d" remote add origin "https://github.com/$2.git"
	echo "$d"
	;;
start)
	[ $# -ge 2 ] || die "usage: start <session> <repo-name> [pho args]"
	s="$1" name="$2"
	shift 2
	[ -x "$BIN" ] || die "no binary at $BIN; run: driver.sh build"
	[ -d "$RUN_DIR/repos/$name" ] || die "no repo dir $name; run: driver.sh repo $name <owner/repo>"
	fakebin
	mkdir -p "$RUN_DIR/cache" "$RUN_DIR/state/pho"
	: >"$RUN_DIR/state/pho/debug.log" # `settle` and `log` read only this run
	tmux kill-session -t "$s" 2>/dev/null || true
	# XDG_CONFIG_HOME stays real: gh keeps its login there and pho reads its
	# config from it. Only the cache and the debug log are redirected.
	tmux new-session -d -s "$s" -x 160 -y 45 \
		"cd '$RUN_DIR/repos/$name' && PATH='$RUN_DIR/fakebin':\$PATH XDG_CACHE_HOME='$RUN_DIR/cache' XDG_STATE_HOME='$RUN_DIR/state' '$BIN' --debug $* 2>'$RUN_DIR/stderr.txt'; echo '[pho exited]'; cat '$RUN_DIR/stderr.txt'; sleep 3600"
	;;
wait)
	[ $# -ge 2 ] || die "usage: wait <session> <text> [secs]"
	s="$1" text="$2" limit="${3:-60}"
	start=$SECONDS
	until tmux capture-pane -t "$s" -p | grep -qF -- "$text"; do
		if tmux capture-pane -t "$s" -p | grep -qF '[pho exited]'; then
			tmux capture-pane -t "$s" -p | tail -5 >&2
			die "pho exited while waiting for: $text"
		fi
		((SECONDS - start < limit)) || die "timed out after ${limit}s waiting for: $text"
		sleep 0.1
	done
	echo "found after $((SECONDS - start))s: $text"
	;;
settle)
	# `pho pr N` loads the diff, then reloads it once the head commit is
	# known; the reload resets the cursor. Wait for at least one load and no
	# new one for 2s.
	limit="${1:-60}" start=$SECONDS last=-1 stable=$SECONDS
	while :; do
		n=$(diffloads)
		n=${n:-0}
		if [ "$n" != "$last" ]; then last=$n stable=$SECONDS; fi
		if [ "$n" -ge 1 ] && ((SECONDS - stable >= 2)); then echo "settled after $((SECONDS - start))s ($n diff loads)"; break; fi
		((SECONDS - start < limit)) || die "diff still loading after ${limit}s"
		sleep 0.2
	done
	;;
keys)
	s="$1"
	shift
	for k in "$@"; do key "$s" "$k"; done
	;;
type)
	tmux send-keys -t "$1" -l "$2"
	sleep "$KEY_DELAY"
	;;
screen) tmux capture-pane -t "$1" -p ;;
cursor)
	c=$(cursorline "$1")
	echo "${c:-(no cursor: the file list has focus, or a diff reload reset it)}"
	;;
status) tmux capture-pane -t "$1" -p | tail -1 ;;
jumpfile)
	s="$1"
	key "$s" h
	key "$s" g
	key "$s" g
	for ((i = 0; i < $2; i++)); do key "$s" j; done
	key "$s" Enter
	;;
goto)
	s="$1" text="$2"
	for ((i = 0; i < 400; i++)); do
		cursorline "$s" | grep -qF -- "$text" && { cursorline "$s"; exit 0; }
		key "$s" j
	done
	die "cursor never reached: $text"
	;;
draft)
	s="$1" extra="$2" body="$3"
	key "$s" " "
	for ((i = 0; i < extra; i++)); do key "$s" j; done
	key "$s" c
	tmux send-keys -t "$s" -l "$body"
	sleep "$KEY_DELAY"
	key "$s" Enter
	;;
opened) cat "$RUN_DIR/opened.txt" 2>/dev/null || true ;;
log) grep -E -- "${1:-.}" "$RUN_DIR/state/pho/debug.log" || true ;;
stop) tmux kill-session -t "$1" 2>/dev/null || true ;;
*) sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//' ;;
esac
