# Contributing

## Layout

- Go implementation at repo root (`cmd/miru`, `internal/...`)
- TypeScript reference under `ts_miru_code/`

## Develop

```bash
go test ./...
go build -o miru ./cmd/miru
```

## Installer

`miru install` must write the Go binary path into agent MCP configs — never `bunx`.

## Parity

Prefer golden / oracle tests under `internal/*/…_test.go` and `internal/oracle/` when changing ranking, tokenization, quantization, or on-disk index formats.
