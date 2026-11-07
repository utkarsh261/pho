#!/usr/bin/env bash
# Time `pho pr <n>` against real GitHub, cold and warm, through the run-pho
# tmux driver. Needs tmux and a logged-in gh; never run in CI.
#
#   scripts/bench/live.sh [owner/repo] [pr] [runs]
set -euo pipefail

repo="${1:-utkarsh261/pho-large-diff-scratch}"
pr="${2:-1}"
runs="${3:-5}"
root="$(git rev-parse --show-toplevel)"
D="$root/.claude/skills/run-pho/driver.sh"
export PHO_RUN_DIR="${PHO_RUN_DIR:-${TMPDIR:-/tmp}/pho-bench-live}"
export PHO_RUN_DIR="${PHO_RUN_DIR%/}"
log="$PHO_RUN_DIR/state/pho/debug.log"
spans='auth resolution|cache init|service wiring|pr load detail|pr load diff|discovery scan'

now_ms() { perl -MTime::HiRes=time -e 'printf "%.0f\n", time*1000'; }
cd "$root"
"$D" build >/dev/null
"$D" repo live "$repo" >/dev/null

gh_ms=()
for _ in $(seq 1 "$runs"); do
	t0=$(now_ms)
	gh auth status --json hosts >/dev/null 2>&1 || gh auth status >/dev/null 2>&1
	gh_ms+=($(($(now_ms) - t0)))
done
echo "gh auth status (on every launch's critical path): ${gh_ms[*]} ms"
echo

# span <name>: durations of a debug-log timer from the last run, comma-separated
span() { grep "\"msg\":\"$1\"" "$log" 2>/dev/null | sed -E 's/.*"duration_ms":([0-9]+).*/\1/' | paste -sd, -; }

printf '%-5s %-4s %8s %8s  %s\n' phase run files-ms diffs spans
for phase in cold warm; do
	for i in $(seq 1 "$runs"); do
		[ "$phase" = cold ] && rm -rf "$PHO_RUN_DIR/cache"
		t0=$(now_ms)
		"$D" start live live pr "$pr"
		until tmux capture-pane -t live -p | grep -qF "Files "; do
			if tmux capture-pane -t live -p | grep -qF '[pho exited]'; then
				tmux capture-pane -t live -p | tail -5 >&2
				exit 1
			fi
			sleep 0.02
		done
		files=$(($(now_ms) - t0))
		"$D" settle >/dev/null
		loads=$({ grep -c '"msg":"pr load diff"' "$log" || true; } | head -1)
		detail=""
		IFS='|' read -ra names <<<"$spans"
		for n in "${names[@]}"; do
			v=$(span "$n")
			[ -n "$v" ] && detail="$detail $n=$v"
		done
		"$D" stop live >/dev/null 2>&1 || true
		printf '%-5s %-4s %8s %8s %s\n' "$phase" "$i" "$files" "$loads" "$detail"
	done
done
