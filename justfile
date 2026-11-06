bin    := "pho"
module := "github.com/utkarsh261/pho"
version := env("VERSION", `git describe --tags --always --dirty 2>/dev/null || echo dev`)

build:
    go build -ldflags="-X main.version={{version}}" -o {{bin}} ./cmd/pho

test:
    go test ./...

test-race:
    go test -race -count=1 ./...

fmt:
    go fmt ./...

vet:
    go vet ./...

clean:
    rm -f {{bin}}

reset:
    go run ./cmd/pho --reset

# every benchmark plus scaling, heap and soak checks; report in bench-out/
bench:
    scripts/bench/compare.sh

# micro benchmarks only, one quick round
bench-quick:
    QUICK=1 scripts/bench/compare.sh

# this tree against another ref, interleaved; report in bench-out/report.md
bench-compare base="main":
    scripts/bench/compare.sh --base {{base}}

# time `pho pr` against real GitHub, cold and warm
bench-live repo="utkarsh261/pho-large-diff-scratch" pr="1" runs="5":
    scripts/bench/live.sh {{repo}} {{pr}} {{runs}}

# CPU profile one benchmark and open it in pprof, e.g. just profile FrameScroll ./internal/ui/app
profile name pkg="./internal/ui/app":
    mkdir -p bench-out
    go test -run '^$' -bench '{{name}}' -benchtime 2s -cpuprofile bench-out/cpu.pprof -o bench-out/profile.test {{pkg}}
    go tool pprof -http=: bench-out/profile.test bench-out/cpu.pprof
