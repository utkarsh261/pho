bin    := "pho"
module := "github.com/utkarsh261/pho"
version := env("VERSION", "dev")

build:
    go build -ldflags="-X main.version={{version}}" -o {{bin}} ./cmd/pho

test:
    go test ./...

vet:
    go vet ./...

clean:
    rm -f {{bin}}

reset:
    go run ./cmd/pho --reset
