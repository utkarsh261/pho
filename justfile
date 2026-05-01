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
