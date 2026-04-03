# CLAUDE.md — ServiceKit

## Quick Reference

ServiceKit is a Go library for production-grade WebSocket, SSE, and gRPC-over-WebSocket infrastructure. See [ARCHITECTURE.md](ARCHITECTURE.md) for design principles and [SUMMARY.md](SUMMARY.md) for a full overview.

## Build & Test

```bash
make test          # go test ./...
make test-race     # go test -race -count=5 -timeout 300s ./...
make vet           # go vet ./...
make install-hooks # install pre-push hook (runs test + test-race)
```

Pre-push hook runs both `make test` and `make test-race` — all tests must pass with `-race` before pushing.

## Key Conventions

- **All writes through Writer** — never write directly to `*websocket.Conn` or `http.ResponseWriter` from user code. Use `SendOutput()` / `Writer.Send()`.
- **Parent OnStart/OnClose** — always call parent when overriding lifecycle hooks (`c.BaseConn.OnStart(conn)`, `c.BaseSSEConn.OnClose()`).
- **SSE requires WriteTimeout=0** — SSE endpoints are long-lived; `http.Server.WriteTimeout` must be 0 or they'll be killed.
- **Nil-safe middleware** — all middleware components are no-ops when nil.
- **Transport/codec separation** — pings/errors are always JSON at transport layer, regardless of data codec.
- **No real gRPC server for tests** — grpcws integration tests use channel-driven test doubles satisfying the stream interfaces. No protobuf codegen needed.

## Gotchas

- **ResponseWriter flush race** — never call `flusher.Flush()` from outside the `conc.Writer` goroutine after `OnStart`. The initial header flush must happen inside `OnStart`, before `conc.NewWriter()` is created. (See v0.0.10 fix.)
- **`BaseSSEConn.Ready()`** — use `<-conn.Ready()` before sending if you get the connection reference before `OnStart` completes (e.g., from `Validate`). The `ready` channel is lazily initialized via `sync.Once`.
- **`timestamppb.Timestamp` in protojson** — serializes as RFC 3339 string (`"1970-01-01T00:16:40Z"`), not `{"seconds":N}`. Tests must account for this.
- **gocurrent dependency** — requires v0.0.13+ for race-free `Writer` and `Reader`. Earlier versions have data races on `isRunning` (Writer) and `closedChan` (Reader).

## Project Docs

| File | Purpose |
|------|---------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Design principles, package structure, component details |
| [SUMMARY.md](SUMMARY.md) | Full overview with recent changes and file reference |
| [LLMGUIDE.md](LLMGUIDE.md) | Templates and quick-reference for LLMs |
| [NEXTSTEPS.md](NEXTSTEPS.md) | TODO list with completed/remaining work |
| [CAPABILITIES.md](CAPABILITIES.md) | Stack component manifest |
| [README.md](README.md) | User-facing tutorial and API guide |
| [UPGRADING.md](UPGRADING.md) | Migration guides (JSONConn → typed BaseConn) |

## Module

```
github.com/panyam/servicekit
```

Packages: `http/`, `grpcws/`, `middleware/`, `auth/`, `clients/typescript/`
