<p align="center">
  <img width="225" height="115" alt="image" src="https://github.com/user-attachments/assets/a29b5502-f081-4b06-ae8e-0ef97fbf9028" />
</p>

<p align="center">
  A TUI for GitHub pull requests.
</p>

<img width="1468" height="889" alt="image" src="https://github.com/user-attachments/assets/f83b174f-afd9-4602-bc98-7355b698cb71" />


## Install

```
go install github.com/utkarsh261/pho/cmd/pho@latest
```

Binary lands in `$(go env GOPATH)/bin/pho`.

Or pin a specific version:

```
go install github.com/utkarsh261/pho/cmd/pho@v0.1.0
```

Add it to the `$PATH`:
```
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
````

## Usage

Right now, pho looks at only the `cwd` and its direct children directories (if they are actually git repos). So if you have some repositories cloned in a directory, you can either open pho in that directory or: 

```
$(go env GOPATH)/bin/pho ~/path/to/dir/containing/all/cloned/repositories
```
or simply start it in the current directory

```
pho
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
