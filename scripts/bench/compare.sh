#!/usr/bin/env bash
# Run pho's benchmarks and write a markdown report.
#
#   scripts/bench/compare.sh                 this tree only
#   scripts/bench/compare.sh --base main     this tree against main
#
# Base and head test binaries are built once, then run in alternating order
# COUNT times so both sides see the same machine state; benchstat-style
# significance needs those repeated samples.
#
# Env: COUNT (default 6), BENCHTIME (1s), QUICK=1 (micro benches only,
# 200ms, one round), OUT (bench-out), BENCH (regexp, default .).
set -euo pipefail

BENCHSTAT=golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e

base=""
while [ $# -gt 0 ]; do
	case "$1" in
	--base) base="$2"; shift 2 ;;
	*) echo "usage: $0 [--base <ref>]" >&2; exit 2 ;;
	esac
done

root="$(git rev-parse --show-toplevel)"
out="${OUT:-$root/bench-out}"
count="${COUNT:-6}"
benchtime="${BENCHTIME:-1s}"
filter="${BENCH:-.}"
quick="${QUICK:-}"
if [ -n "$quick" ]; then
	count=1
	benchtime=200ms
fi
pin=()
if command -v taskset >/dev/null 2>&1; then
	pin=(taskset -c 0,1)
fi

rm -rf "$out"
mkdir -p "$out/bin"
work="$(mktemp -d)"
cleanup() {
	if [ -n "$base" ] && [ -d "$work/base" ]; then
		git -C "$root" worktree remove --force "$work/base" >/dev/null 2>&1 || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT

# packages with benchmarks, relative to the module root
bench_pkgs() {
	(cd "$1" && grep -rl --include='*_test.go' '^func Benchmark' . | xargs -n1 dirname | sort -u)
}

# build <side> <tree>: one test binary per package, plus the health checks
build() {
	local side="$1" tree="$2"
	mkdir -p "$out/bin/$side"
	for pkg in $(bench_pkgs "$tree"); do
		if [ -n "$quick" ] && { [ "$pkg" = ./cmd/pho ] || [ "$pkg" = ./internal/ui/app ]; }; then
			continue
		fi
		(cd "$tree" && go test -c -o "$out/bin/$side/$(echo "${pkg#./}" | tr / _).test" "$pkg")
	done
	if [ -z "$quick" ] && grep -q '^//go:build bench' "$tree/internal/ui/app/health_bench_test.go" 2>/dev/null; then
		(cd "$tree" && go test -c -tags bench -o "$out/bin/$side/health.test" ./internal/ui/app)
	fi
}

# round <side> <tree>: one pass over every benchmark binary
round() {
	local side="$1" tree="$2"
	for bin in "$out/bin/$side"/*.test; do
		local name pkg bt="$benchtime"
		name="$(basename "$bin" .test)"
		[ "$name" = health ] && continue
		pkg="$tree/$(echo "$name" | tr _ /)"
		# a launch is one op; repeating it inside a run only measures the cache
		[ "$name" = cmd_pho ] && bt=1x
		(cd "$pkg" && ${pin[@]+"${pin[@]}"} "$bin" -test.run '^$' -test.bench "$filter" -test.benchmem \
			-test.benchtime "$bt" -test.count 1) | grep -E '^(Benchmark|goos|goarch|pkg|cpu)' >>"$out/$side.txt" || true
	done
}

health() {
	local side="$1" tree="$2"
	[ -x "$out/bin/$side/health.test" ] || return 0
	(cd "$tree/internal/ui/app" && ${pin[@]+"${pin[@]}"} "$out/bin/$side/health.test" -test.run 'TestScaling|TestHeapPlateau|TestMonkeySoak' -test.v) |
		grep -E 'SCALING|HEAP|SOAK' >>"$out/$side.txt" || true
}

echo "building head ($root)" >&2
build head "$root"
if [ -n "$base" ]; then
	echo "building base ($base)" >&2
	git -C "$root" worktree add --detach "$work/base" "$base" >/dev/null
	build base "$work/base"
fi

for i in $(seq 1 "$count"); do
	echo "round $i/$count" >&2
	if [ -n "$base" ] && [ $((i % 2)) -eq 1 ]; then
		round base "$work/base"
		round head "$root"
	else
		round head "$root"
		[ -n "$base" ] && round base "$work/base"
	fi
done
health head "$root"
[ -n "$base" ] && health base "$work/base"

head_ref="$(git -C "$root" rev-parse --short HEAD)"
if ! git -C "$root" diff --quiet; then
	head_ref="$head_ref+dirty"
fi
args=(-head "$out/head.txt" -head-ref "$head_ref" -json "$out/head.json")
if [ -n "$base" ]; then
	args+=(-base "$out/base.txt" -base-ref "$base@$(git -C "$root" rev-parse --short "$base")")
	(cd "$out" && go run "$BENCHSTAT" base=base.txt head=head.txt) >"$out/benchstat.txt" 2>/dev/null || true
else
	(cd "$out" && go run "$BENCHSTAT" head=head.txt) >"$out/benchstat.txt" 2>/dev/null || true
fi
(cd "$root" && go run ./scripts/bench/report "${args[@]}") >"$out/report.md"
{
	echo
	echo "<details><summary>Full benchstat</summary>"
	echo
	echo '```'
	cat "$out/benchstat.txt"
	echo '```'
	echo "</details>"
} >>"$out/report.md"
echo "wrote $out/report.md" >&2
