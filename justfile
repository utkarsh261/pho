bin := "git-term"
module := "github.com/utk/git-term"
version := env("VERSION", "dev")

build:
    go build -ldflags="-X main.version={{version}}" -o {{bin}} ./cmd/git-term

test:
    go test ./...

vet:
    go vet ./...

clean:
    rm -f {{bin}}

reset:
    go run ./cmd/git-term --reset
