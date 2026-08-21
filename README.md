# Miru Code

Hybrid semantic + keyword code search for coding agents. This repository is the **Go** implementation (`github.com/takara-ai/miru-code`). 
## Install / build

```bash
go build -o miru ./cmd/miru
go install ./cmd/miru   # optional: onto PATH
```

Requires Go 1.25+ and **CGO** (tree-sitter AST chunking). Set `MIRU_AST_CHUNKING=0` to fall back to structural/line chunking without relying on grammars at runtime.

## Quick start

```bash
miru setup --device          # or: miru setup --key TOKEN
miru install --yes --all    # writes MCP configs pointing at this Go binary
miru search "auth middleware" .
```

With no known subcommand, `miru` serves MCP over stdio (line-delimited JSON-RPC).

## Agent MCP config

`miru install` writes the **absolute path** of the Go `miru` binary as the MCP `command` (never `bunx`).

## Commands

| Command | Description |
|---------|-------------|
| _(default)_ | MCP server (stdio) |
| `search` / `locate` / `expand` / `find-related` | CLI search tools |
| `setup` | Takara / SageMaker credentials |
| `install` / `uninstall` | Agent MCP + instructions |
| `init --agent` | Project-local sub-agent file |
| `hook-guard` | Search-policy hook (exit 2 = deny for Cursor) |
| `-v` / `help` | Version / help |

Exit codes: `0` success, `1` usage/error, `2` hook deny.

## MCP tools

`search`, `locate`, `expand`, `find_related`, `auth` — results are always `{ content: [{ type: "text", text }] }`.

## Index cache

On-disk layout matches the TS port for interoperability:

```
{cache}/{sha256(sourceKey)}/index/
  bm25_index.json
  chunks.json
  metadata.json
  semantic_index/{meta.json,codes.bin,scales.bin}   # int8 default
```

## Credentials

- macOS: `~/Library/Application Support/miru/credentials.json`
- Linux: `~/.config/miru/credentials.json`
- Windows: `%APPDATA%/miru/credentials.json`

Override with `TAKARA_API_KEY` or `MIRU_CREDENTIALS_DIR`.

## Development

```bash
make test
make build
go test ./internal/oracle/...   # parity/oracle suite
```

See [CONTRIBUTING.md](CONTRIBUTING.md).
