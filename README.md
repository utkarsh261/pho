# pho

A terminal UI for GitHub pull requests.

## Install

```
go install github.com/utkarsh261/pho/cmd/pho@latest
```

Binary lands in `$(go env GOPATH)/bin/pho`.

Or pin a specific version:

```
go install github.com/utkarsh261/pho/cmd/pho@v0.1.0
```

## Usage

Right now, pho looks at only the repositories cloned in a parent directory, you can either open pho in that directory or:

```
$(go env GOPATH)/bin/pho ~/path/to/dir/containing/all/cloned/repositories
```

## Requirements

- Go 1.25+
- Git
- [GitHub CLI (`gh`)](https://cli.github.com) — run `gh auth login` to authenticate

## Build

```
go build -o pho ./cmd/pho
```

With `just`:

```
just build
```

## Run

```
./pho
```

Flags:

| Flag | Description |
|------|-------------|
| `--version` | Print version and exit |
| `--debug` | Enable debug logging |
| `--reset` | Clear all caches and exit |
| `--config <path>` | Path to config file |
| `--root <dir>` | Root directory to scan for git repos (default `.`) |

## Test

```
go test ./...
```

With `just`:

```
just test
```

## Vet

```
go vet ./...
```

With `just`:

```
just vet
```

## Logs

```
tail -f ~/.local/state/pho/debug.log
```
