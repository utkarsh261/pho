# git-term

## Requirements

- Go 1.25+
- `just` (optional, for running justfile recipes)

## Build

```
go build -o git-term ./cmd/git-term
```

With `just`:

```
just build
```

## Run

```
./git-term
```

Flags:

| Flag | Description |
|------|-------------|
| `--version` | Print version and exit |
| `--debug` | Enable debug logging |
| `--reset` | Clear all caches and exit |
| `--config <path>` | Path to config file |
| `--root <dir>` | Root directory to scan for git repos (default `.`) |

With `just`:

```
just run          # runs with current settings
just reset        # clears caches
```

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
